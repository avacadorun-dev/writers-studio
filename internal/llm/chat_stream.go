package llm

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

const maxProviderStreamLineBytes = 1 << 20

type chatStreamToolCall struct {
	ID        string
	Type      string
	Name      string
	Arguments strings.Builder
}

type chatStreamAccumulator struct {
	responseID       string
	role             string
	content          strings.Builder
	reasoning        strings.Builder
	reasoningContent strings.Builder
	toolCalls        map[int]*chatStreamToolCall
	reasoningDetails map[int]map[string]any
	sawData          bool
	sawTerminal      bool
}

type chatStreamEnvelope struct {
	ID      string          `json:"id"`
	Error   json.RawMessage `json:"error"`
	Choices []struct {
		Index        int             `json:"index"`
		Delta        json.RawMessage `json:"delta"`
		FinishReason json.RawMessage `json:"finish_reason"`
	} `json:"choices"`
}

type chatStreamDelta struct {
	Role             string            `json:"role"`
	Content          json.RawMessage   `json:"content"`
	Reasoning        string            `json:"reasoning"`
	ReasoningContent string            `json:"reasoning_content"`
	ReasoningDetails []json.RawMessage `json:"reasoning_details"`
	ToolCalls        []struct {
		Index    int    `json:"index"`
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls"`
}

// parseChatCompletionStream consumes OpenAI-compatible SSE while retaining the
// assistant message needed for the next tool-result turn. OpenRouter emits SSE
// comments while a model is thinking; consuming those keepalives avoids the
// upstream idle cutoff that affects large non-streaming tool calls.
func parseChatCompletionStream(reader io.Reader) (Completion, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), maxProviderStreamLineBytes)
	accumulator := chatStreamAccumulator{
		toolCalls:        make(map[int]*chatStreamToolCall),
		reasoningDetails: make(map[int]map[string]any),
	}
	var eventData []string
	consumed := 0
	processEvent := func() error {
		if len(eventData) == 0 {
			return nil
		}
		payload := strings.Join(eventData, "\n")
		eventData = eventData[:0]
		if payload == "[DONE]" {
			accumulator.sawTerminal = true
			return nil
		}
		return accumulator.add([]byte(payload))
	}
	for scanner.Scan() {
		line := scanner.Text()
		consumed += len(line) + 1
		if consumed > maxProviderResponseBytes {
			return Completion{}, fmt.Errorf("%w: response too large", ErrProviderResponse)
		}
		if line == "" {
			if err := processEvent(); err != nil {
				return Completion{}, err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			value := strings.TrimPrefix(line, "data:")
			if strings.HasPrefix(value, " ") {
				value = value[1:]
			}
			eventData = append(eventData, value)
		}
	}
	if err := scanner.Err(); err != nil {
		if isProviderTimeout(err) {
			return Completion{}, safeProviderTimeoutError()
		}
		return Completion{}, fmt.Errorf("%w: stream read failed", ErrProviderResponse)
	}
	if err := processEvent(); err != nil {
		return Completion{}, err
	}
	return accumulator.completion()
}

func (accumulator *chatStreamAccumulator) add(data []byte) error {
	var envelope chatStreamEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return ErrProviderResponse
	}
	if len(envelope.Error) > 0 && string(envelope.Error) != "null" {
		return decodeProviderStreamError(envelope.Error)
	}
	if envelope.ID != "" {
		accumulator.responseID = envelope.ID
	}
	if len(envelope.Choices) == 0 {
		// Usage- and metadata-only chunks are valid before [DONE].
		return nil
	}
	for _, choice := range envelope.Choices {
		if choice.Index != 0 || len(choice.Delta) == 0 || string(choice.Delta) == "null" {
			continue
		}
		accumulator.sawData = true
		if len(choice.FinishReason) > 0 && string(choice.FinishReason) != "null" {
			accumulator.sawTerminal = true
		}
		var delta chatStreamDelta
		if err := json.Unmarshal(choice.Delta, &delta); err != nil {
			return ErrProviderResponse
		}
		if delta.Role != "" {
			accumulator.role = delta.Role
		}
		if len(delta.Content) > 0 && string(delta.Content) != "null" {
			var text string
			if json.Unmarshal(delta.Content, &text) == nil {
				accumulator.content.WriteString(text)
			}
		}
		accumulator.reasoning.WriteString(delta.Reasoning)
		accumulator.reasoningContent.WriteString(delta.ReasoningContent)
		for _, raw := range delta.ReasoningDetails {
			if err := accumulator.addReasoningDetail(raw); err != nil {
				return err
			}
		}
		for _, deltaCall := range delta.ToolCalls {
			call := accumulator.toolCalls[deltaCall.Index]
			if call == nil {
				call = &chatStreamToolCall{Type: "function"}
				accumulator.toolCalls[deltaCall.Index] = call
			}
			if deltaCall.ID != "" {
				call.ID = deltaCall.ID
			}
			if deltaCall.Type != "" {
				call.Type = deltaCall.Type
			}
			if deltaCall.Function.Name != "" {
				call.Name = appendStreamFragment(call.Name, deltaCall.Function.Name)
			}
			call.Arguments.WriteString(deltaCall.Function.Arguments)
		}
	}
	return nil
}

func appendStreamFragment(current string, incoming string) string {
	if incoming == "" || incoming == current {
		return current
	}
	return current + incoming
}

func (accumulator *chatStreamAccumulator) addReasoningDetail(raw json.RawMessage) error {
	var detail map[string]any
	if err := json.Unmarshal(raw, &detail); err != nil {
		return ErrProviderResponse
	}
	index := len(accumulator.reasoningDetails)
	if numeric, ok := detail["index"].(float64); ok && numeric >= 0 {
		index = int(numeric)
	}
	existing := accumulator.reasoningDetails[index]
	if existing == nil {
		existing = make(map[string]any)
		accumulator.reasoningDetails[index] = existing
	}
	for key, value := range detail {
		incoming, isString := value.(string)
		current, hasCurrent := existing[key].(string)
		if isString && hasCurrent && key != "id" && key != "type" && key != "format" {
			existing[key] = current + incoming
			continue
		}
		if !hasCurrent || !isString || incoming != "" {
			existing[key] = value
		}
	}
	return nil
}

func (accumulator *chatStreamAccumulator) completion() (Completion, error) {
	if accumulator.responseID == "" || !accumulator.sawData || !accumulator.sawTerminal {
		return Completion{}, ErrProviderResponse
	}
	role := accumulator.role
	if role == "" {
		role = "assistant"
	}
	message := map[string]any{"role": role, "content": nil}
	if accumulator.content.Len() > 0 {
		message["content"] = accumulator.content.String()
	}
	if accumulator.reasoning.Len() > 0 {
		message["reasoning"] = accumulator.reasoning.String()
	}
	if accumulator.reasoningContent.Len() > 0 {
		message["reasoning_content"] = accumulator.reasoningContent.String()
	}
	if len(accumulator.reasoningDetails) > 0 {
		indexes := sortedIntKeys(accumulator.reasoningDetails)
		details := make([]map[string]any, 0, len(indexes))
		for _, index := range indexes {
			details = append(details, accumulator.reasoningDetails[index])
		}
		message["reasoning_details"] = details
	}
	if len(accumulator.toolCalls) > 0 {
		indexes := sortedIntKeys(accumulator.toolCalls)
		calls := make([]map[string]any, 0, len(indexes))
		for _, index := range indexes {
			call := accumulator.toolCalls[index]
			calls = append(calls, map[string]any{
				"id": call.ID, "type": call.Type,
				"function": map[string]any{
					"name": call.Name, "arguments": call.Arguments.String(),
				},
			})
		}
		message["tool_calls"] = calls
	}
	envelope, err := json.Marshal(map[string]any{
		"id":      accumulator.responseID,
		"choices": []any{map[string]any{"message": message}},
	})
	if err != nil {
		return Completion{}, ErrProviderResponse
	}
	return parseChatCompletion(envelope)
}

func sortedIntKeys[T any](values map[int]T) []int {
	keys := make([]int, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	return keys
}

func decodeProviderStreamError(raw json.RawMessage) error {
	var payload struct {
		Message  string          `json:"message"`
		Code     json.RawMessage `json:"code"`
		Metadata struct {
			ErrorType    string          `json:"error_type"`
			ProviderCode json.RawMessage `json:"provider_code"`
			ProviderName string          `json:"provider_name"`
		} `json:"metadata"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return ErrProviderResponse
	}
	result := &ProviderHTTPError{
		StatusCode:   http.StatusBadGateway,
		Message:      safeProviderErrorText(payload.Message),
		ErrorType:    safeProviderErrorText(payload.Metadata.ErrorType),
		ProviderName: safeProviderErrorText(payload.Metadata.ProviderName),
	}
	result.ProviderCode, result.StatusCode = providerStreamCode(payload.Code, result.StatusCode)
	if len(payload.Metadata.ProviderCode) > 0 {
		result.ProviderCode, result.StatusCode = providerStreamCode(
			payload.Metadata.ProviderCode, result.StatusCode,
		)
	}
	return result
}

func providerStreamCode(raw json.RawMessage, fallbackStatus int) (string, int) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", fallbackStatus
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		if numeric, err := strconv.Atoi(value); err == nil && numeric >= 400 && numeric <= 599 {
			return safeProviderErrorText(value), numeric
		}
		return safeProviderErrorText(value), fallbackStatus
	}
	var numeric json.Number
	if json.Unmarshal(raw, &numeric) == nil {
		value = numeric.String()
		if status, err := strconv.Atoi(value); err == nil && status >= 400 && status <= 599 {
			return value, status
		}
		return value, fallbackStatus
	}
	return "", fallbackStatus
}

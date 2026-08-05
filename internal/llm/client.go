package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"writers.studio/internal/assistant"
)

const maxProviderResponseBytes = 8 << 20
const maxProviderErrorBytes = 64 << 10

var (
	ErrProviderUnavailable = errors.New("LLM provider is unavailable")
	ErrProviderResponse    = errors.New("LLM provider returned an invalid response")
)

func isProviderTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var networkError net.Error
	return errors.As(err, &networkError) && networkError.Timeout()
}

func safeProviderTimeoutError() error {
	return fmt.Errorf("%w: %w", ErrProviderUnavailable, context.DeadlineExceeded)
}

type ProviderHTTPError struct {
	StatusCode   int
	ProviderCode string
	ErrorType    string
	ProviderName string
	Message      string
}

func (e *ProviderHTTPError) Error() string {
	detail := ""
	if e.ProviderCode != "" {
		detail = " code=" + e.ProviderCode
	}
	if e.ErrorType != "" {
		detail += " type=" + e.ErrorType
	}
	if e.ProviderName != "" {
		detail += " provider=" + e.ProviderName
	}
	if e.Message != "" {
		detail += " message=" + e.Message
	}
	return fmt.Sprintf("%s: HTTP %d%s", ErrProviderUnavailable, e.StatusCode, detail)
}

func (*ProviderHTTPError) Unwrap() error {
	return ErrProviderUnavailable
}

type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type Request struct {
	Instructions string
	Input        []json.RawMessage
	Tools        []ToolDefinition
	ToolChoice   string
}

type Completion struct {
	ResponseID   string
	Text         string
	ToolCalls    []ToolCall
	Continuation []json.RawMessage
}

type Provider interface {
	Complete(context.Context, Request) (Completion, error)
}

type OpenAICompatible struct {
	baseURL    *url.URL
	apiKey     string
	model      string
	dialect    assistant.ProviderDialect
	httpClient *http.Client
	headers    map[string]string
}

type OpenAICompatibleConfig struct {
	BaseURL    *url.URL
	APIKey     string
	Model      string
	Dialect    assistant.ProviderDialect
	HTTPClient *http.Client
	Headers    map[string]string
}

func NewOpenAICompatible(config OpenAICompatibleConfig) (*OpenAICompatible, error) {
	if config.BaseURL == nil || strings.TrimSpace(config.APIKey) == "" ||
		strings.TrimSpace(config.Model) == "" || config.HTTPClient == nil {
		return nil, errors.New("incomplete OpenAI-compatible provider configuration")
	}
	if config.Dialect != assistant.DialectResponses &&
		config.Dialect != assistant.DialectChatCompletions {
		return nil, errors.New("provider dialect must be resolved")
	}
	headers := make(map[string]string, len(config.Headers))
	for name, value := range config.Headers {
		canonical := http.CanonicalHeaderKey(strings.TrimSpace(name))
		if canonical == "" || strings.EqualFold(canonical, "Authorization") ||
			strings.EqualFold(canonical, "Host") || strings.ContainsAny(value, "\r\n") {
			return nil, errors.New("provider contains a forbidden custom header")
		}
		headers[canonical] = strings.TrimSpace(value)
	}
	clonedURL := *config.BaseURL
	clonedURL.Path = strings.TrimRight(clonedURL.Path, "/")
	return &OpenAICompatible{
		baseURL: &clonedURL, apiKey: strings.TrimSpace(config.APIKey),
		model: strings.TrimSpace(config.Model), dialect: config.Dialect,
		httpClient: config.HTTPClient, headers: headers,
	}, nil
}

func (p *OpenAICompatible) Complete(ctx context.Context, request Request) (Completion, error) {
	wireRequest, internalNames, err := prepareOpenAIWireRequest(request)
	if err != nil {
		return Completion{}, err
	}
	var endpoint string
	var payload any
	switch p.dialect {
	case assistant.DialectResponses:
		endpoint = "/responses"
		payload = p.responsesPayload(wireRequest)
	case assistant.DialectChatCompletions:
		endpoint = "/chat/completions"
		payload = p.chatPayload(wireRequest)
	default:
		return Completion{}, errors.New("provider dialect is not supported")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Completion{}, fmt.Errorf("encode provider request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.baseURL.String()+endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return Completion{}, fmt.Errorf("create provider request: %w", err)
	}
	for name, value := range p.headers {
		httpRequest.Header.Set(name, value)
	}
	// Transport-owned headers are applied last so a custom connection cannot
	// silently disable streaming or replace the request media type.
	httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	if p.dialect == assistant.DialectChatCompletions {
		httpRequest.Header.Set("Accept", "text/event-stream, application/json")
	} else {
		httpRequest.Header.Set("Accept", "application/json")
	}
	response, err := p.httpClient.Do(httpRequest)
	if err != nil {
		if isProviderTimeout(err) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			// Preserve only the safe timeout class. A url.Error can contain the
			// provider endpoint, so it must not cross the application boundary.
			return Completion{}, safeProviderTimeoutError()
		}
		return Completion{}, fmt.Errorf("%w: request failed", ErrProviderUnavailable)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Completion{}, decodeProviderHTTPError(response)
	}
	if p.dialect == assistant.DialectChatCompletions &&
		strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		completion, streamErr := parseChatCompletionStream(response.Body)
		if streamErr != nil {
			return Completion{}, streamErr
		}
		for index := range completion.ToolCalls {
			internalName, ok := internalNames[completion.ToolCalls[index].Name]
			if !ok {
				return Completion{}, ErrProviderResponse
			}
			completion.ToolCalls[index].Name = internalName
		}
		return completion, nil
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxProviderResponseBytes+1))
	if err != nil {
		if isProviderTimeout(err) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Completion{}, safeProviderTimeoutError()
		}
		return Completion{}, fmt.Errorf("%w: read failed", ErrProviderResponse)
	}
	if len(data) > maxProviderResponseBytes {
		return Completion{}, fmt.Errorf("%w: response too large", ErrProviderResponse)
	}
	var completion Completion
	if p.dialect == assistant.DialectResponses {
		completion, err = parseResponsesCompletion(data)
	} else {
		completion, err = parseChatCompletion(data)
	}
	if err != nil {
		return Completion{}, err
	}
	for index := range completion.ToolCalls {
		internalName, ok := internalNames[completion.ToolCalls[index].Name]
		if !ok {
			return Completion{}, ErrProviderResponse
		}
		completion.ToolCalls[index].Name = internalName
	}
	return completion, nil
}

// prepareOpenAIWireRequest keeps the MCP namespace readable inside Writers Studio while
// satisfying OpenAI's function-name alphabet on the wire. OpenAI rejects dots when a
// tool call is replayed in conversation history, even when a gateway accepted the same
// dotted name in the initial declaration.
func prepareOpenAIWireRequest(request Request) (Request, map[string]string, error) {
	wire := request
	wire.Tools = make([]ToolDefinition, len(request.Tools))
	internalNames := make(map[string]string, len(request.Tools))
	wireByInternal := make(map[string]string, len(request.Tools))
	for index, tool := range request.Tools {
		wireName := strings.ReplaceAll(tool.Name, ".", "_")
		if !validOpenAIToolName(wireName) {
			return Request{}, nil, fmt.Errorf("%w: invalid tool name", ErrProviderResponse)
		}
		if existing, exists := internalNames[wireName]; exists && existing != tool.Name {
			return Request{}, nil, fmt.Errorf("%w: colliding tool names", ErrProviderResponse)
		}
		internalNames[wireName] = tool.Name
		wireByInternal[tool.Name] = wireName
		wire.Tools[index] = tool
		wire.Tools[index].Name = wireName
	}
	if request.ToolChoice != "" {
		wireName, ok := wireByInternal[request.ToolChoice]
		if !ok {
			return Request{}, nil, fmt.Errorf("%w: tool choice is not declared", ErrProviderResponse)
		}
		wire.ToolChoice = wireName
	}
	return wire, internalNames, nil
}

func validOpenAIToolName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, character := range name {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func decodeProviderHTTPError(response *http.Response) error {
	result := &ProviderHTTPError{StatusCode: response.StatusCode}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxProviderErrorBytes))
	if err != nil {
		return result
	}
	var envelope struct {
		Error struct {
			Message  string          `json:"message"`
			Code     json.RawMessage `json:"code"`
			Metadata struct {
				ErrorType    string          `json:"error_type"`
				ProviderCode json.RawMessage `json:"provider_code"`
				ProviderName string          `json:"provider_name"`
			} `json:"metadata"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return result
	}
	result.Message = safeProviderErrorText(envelope.Error.Message)
	result.ErrorType = safeProviderErrorText(envelope.Error.Metadata.ErrorType)
	result.ProviderName = safeProviderErrorText(envelope.Error.Metadata.ProviderName)
	if len(envelope.Error.Code) > 0 {
		var code string
		if json.Unmarshal(envelope.Error.Code, &code) == nil {
			result.ProviderCode = safeProviderErrorText(code)
		} else {
			var numeric json.Number
			if json.Unmarshal(envelope.Error.Code, &numeric) == nil {
				result.ProviderCode = numeric.String()
			}
		}
	}
	if len(envelope.Error.Metadata.ProviderCode) > 0 {
		var providerCode string
		if json.Unmarshal(envelope.Error.Metadata.ProviderCode, &providerCode) == nil {
			result.ProviderCode = safeProviderErrorText(providerCode)
		} else {
			var numeric json.Number
			if json.Unmarshal(envelope.Error.Metadata.ProviderCode, &numeric) == nil {
				result.ProviderCode = numeric.String()
			}
		}
	}
	return result
}

func safeProviderErrorText(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > 500 {
		value = string(runes[:500])
	}
	return value
}

func (p *OpenAICompatible) responsesPayload(request Request) map[string]any {
	tools := make([]map[string]any, 0, len(request.Tools))
	for _, tool := range request.Tools {
		tools = append(tools, map[string]any{
			"type":        "function",
			"name":        tool.Name,
			"description": tool.Description,
			"parameters":  tool.Parameters,
			"strict":      true,
		})
	}
	payload := map[string]any{
		"model":               p.model,
		"instructions":        request.Instructions,
		"input":               rawItems(request.Input),
		"tools":               tools,
		"parallel_tool_calls": false,
		"stream":              false,
	}
	if request.ToolChoice != "" {
		payload["tool_choice"] = map[string]any{
			"type": "function", "name": request.ToolChoice,
		}
	}
	return payload
}

func (p *OpenAICompatible) chatPayload(request Request) map[string]any {
	messages := make([]any, 0, len(request.Input)+1)
	if strings.TrimSpace(request.Instructions) != "" {
		messages = append(messages, map[string]any{
			"role": "system", "content": request.Instructions,
		})
	}
	messages = append(messages, rawItems(request.Input)...)
	tools := make([]map[string]any, 0, len(request.Tools))
	for _, tool := range request.Tools {
		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.Parameters,
				"strict":      true,
			},
		})
	}
	toolChoice := any("auto")
	if request.ToolChoice != "" {
		toolChoice = map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": request.ToolChoice,
			},
		}
	}
	return map[string]any{
		"model":               p.model,
		"messages":            messages,
		"tools":               tools,
		"tool_choice":         toolChoice,
		"parallel_tool_calls": false,
		"stream":              true,
	}
}

func rawItems(items []json.RawMessage) []any {
	result := make([]any, 0, len(items))
	for _, item := range items {
		result = append(result, item)
	}
	return result
}

func UserInput(text string) json.RawMessage {
	return MessageInput("user", text)
}

func MessageInput(role string, text string) json.RawMessage {
	data, _ := json.Marshal(map[string]string{"role": role, "content": text})
	return data
}

func ToolOutput(
	dialect assistant.ProviderDialect,
	callID string,
	output any,
) (json.RawMessage, error) {
	encodedOutput, err := json.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("encode tool output: %w", err)
	}
	var item any
	switch dialect {
	case assistant.DialectResponses:
		item = map[string]any{
			"type": "function_call_output", "call_id": callID, "output": string(encodedOutput),
		}
	case assistant.DialectChatCompletions:
		item = map[string]any{
			"role": "tool", "tool_call_id": callID, "content": string(encodedOutput),
		}
	default:
		return nil, errors.New("provider dialect is not supported")
	}
	return json.Marshal(item)
}

// CompactToolCallArguments removes already-staged long-form prose from provider
// replay while preserving the original call IDs and all opaque provider state.
// The matching tool result carries the accumulated word count and a short tail,
// so the next step can continue coherently without paying to resend every chunk.
func CompactToolCallArguments(
	dialect assistant.ProviderDialect,
	continuation []json.RawMessage,
	replacements map[string]json.RawMessage,
) ([]json.RawMessage, error) {
	if len(replacements) == 0 {
		return append([]json.RawMessage(nil), continuation...), nil
	}
	result := make([]json.RawMessage, 0, len(continuation))
	for _, raw := range continuation {
		var item map[string]any
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, ErrProviderResponse
		}
		switch dialect {
		case assistant.DialectResponses:
			callID, _ := item["call_id"].(string)
			if replacement, exists := replacements[callID]; exists && item["type"] == "function_call" {
				item["arguments"] = string(replacement)
			}
		case assistant.DialectChatCompletions:
			calls, _ := item["tool_calls"].([]any)
			for _, rawCall := range calls {
				call, _ := rawCall.(map[string]any)
				callID, _ := call["id"].(string)
				replacement, exists := replacements[callID]
				if !exists {
					continue
				}
				function, _ := call["function"].(map[string]any)
				if function == nil {
					return nil, ErrProviderResponse
				}
				function["arguments"] = string(replacement)
			}
		default:
			return nil, errors.New("provider dialect is not supported")
		}
		encoded, err := json.Marshal(item)
		if err != nil {
			return nil, ErrProviderResponse
		}
		result = append(result, encoded)
	}
	return result, nil
}

type responsesEnvelope struct {
	ID     string            `json:"id"`
	Output []json.RawMessage `json:"output"`
}

type responsesOutputItem struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func parseResponsesCompletion(data []byte) (Completion, error) {
	var envelope responsesEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil || envelope.ID == "" {
		return Completion{}, ErrProviderResponse
	}
	completion := Completion{
		ResponseID: envelope.ID, Continuation: append([]json.RawMessage(nil), envelope.Output...),
	}
	var textParts []string
	for _, raw := range envelope.Output {
		if len(raw) == 0 || string(raw) == "null" {
			return Completion{}, ErrProviderResponse
		}
		var item responsesOutputItem
		if err := json.Unmarshal(raw, &item); err != nil {
			return Completion{}, ErrProviderResponse
		}
		switch item.Type {
		case "message":
			for _, content := range item.Content {
				if content.Type == "output_text" && content.Text != "" {
					textParts = append(textParts, content.Text)
				}
			}
		case "function_call":
			if item.CallID == "" || item.Name == "" || !json.Valid([]byte(item.Arguments)) {
				return Completion{}, ErrProviderResponse
			}
			completion.ToolCalls = append(completion.ToolCalls, ToolCall{
				ID: item.CallID, Name: item.Name, Arguments: json.RawMessage(item.Arguments),
			})
		}
	}
	completion.Text = strings.Join(textParts, "\n")
	return completion, nil
}

type chatEnvelope struct {
	ID      string `json:"id"`
	Choices []struct {
		Message json.RawMessage `json:"message"`
	} `json:"choices"`
}

type chatMessage struct {
	Content   any `json:"content"`
	ToolCalls []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls"`
}

func parseChatCompletion(data []byte) (Completion, error) {
	var envelope chatEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil || envelope.ID == "" ||
		len(envelope.Choices) == 0 || len(envelope.Choices[0].Message) == 0 ||
		string(envelope.Choices[0].Message) == "null" {
		return Completion{}, ErrProviderResponse
	}
	var message chatMessage
	if err := json.Unmarshal(envelope.Choices[0].Message, &message); err != nil {
		return Completion{}, ErrProviderResponse
	}
	completion := Completion{
		ResponseID: envelope.ID,
		Text:       chatContentText(message.Content),
		Continuation: []json.RawMessage{
			append(json.RawMessage(nil), envelope.Choices[0].Message...),
		},
	}
	for _, call := range message.ToolCalls {
		if call.Type != "function" || call.ID == "" || call.Function.Name == "" ||
			!json.Valid([]byte(call.Function.Arguments)) {
			return Completion{}, ErrProviderResponse
		}
		completion.ToolCalls = append(completion.ToolCalls, ToolCall{
			ID: call.ID, Name: call.Function.Name,
			Arguments: json.RawMessage(call.Function.Arguments),
		})
	}
	return completion, nil
}

func chatContentText(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []any:
		var parts []string
		for _, rawPart := range value {
			part, ok := rawPart.(map[string]any)
			if !ok || part["type"] != "text" {
				continue
			}
			if text, ok := part["text"].(string); ok && text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

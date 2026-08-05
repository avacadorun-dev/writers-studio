package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"writers.studio/internal/assistant"
)

func strictTestTool() ToolDefinition {
	return ToolDefinition{
		Name: "project.get_overview", Description: "Read overview",
		Parameters: map[string]any{
			"type": "object", "properties": map[string]any{},
			"required": []string{}, "additionalProperties": false,
		},
	}
}

func newTestProvider(
	t *testing.T,
	dialect assistant.ProviderDialect,
	handler http.HandlerFunc,
) *OpenAICompatible {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewOpenAICompatible(OpenAICompatibleConfig{
		BaseURL: baseURL, APIKey: "secret", Model: "writer",
		Dialect: dialect, HTTPClient: server.Client(), Headers: map[string]string{"X-Test": "yes"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func TestResponsesCompletionAndPayload(t *testing.T) {
	t.Parallel()
	provider := newTestProvider(t, assistant.DialectResponses, func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path != "/responses" ||
			request.Header.Get("Authorization") != "Bearer secret" ||
			request.Header.Get("X-Test") != "yes" {
			t.Errorf("unexpected request: %s, %v", request.URL.Path, request.Header)
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload["parallel_tool_calls"] != false || payload["stream"] != false {
			t.Errorf("unsafe provider flags: %+v", payload)
		}
		choice := payload["tool_choice"].(map[string]any)
		if choice["type"] != "function" || choice["name"] != "project_get_overview" {
			t.Errorf("forced Responses tool choice is missing: %+v", choice)
		}
		tools := payload["tools"].([]any)
		tool := tools[0].(map[string]any)
		if tool["strict"] != true || tool["name"] != "project_get_overview" {
			t.Errorf("unexpected strict tool: %+v", tool)
		}
		_, _ = io.WriteString(response, `{
			"id":"resp_1",
			"output":[
				{"type":"reasoning","id":"reason_1","summary":[]},
				{"type":"message","content":[{"type":"output_text","text":"Проверяю проект."}]},
				{"type":"function_call","call_id":"call_1","name":"project_get_overview","arguments":"{}"}
			]
		}`)
	})
	result, err := provider.Complete(context.Background(), Request{
		Instructions: "System", Input: []json.RawMessage{UserInput("Что в проекте?")},
		Tools: []ToolDefinition{strictTestTool()}, ToolChoice: "project.get_overview",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ResponseID != "resp_1" || result.Text != "Проверяю проект." ||
		len(result.ToolCalls) != 1 || len(result.Continuation) != 3 {
		t.Fatalf("unexpected completion: %+v", result)
	}
	if result.ToolCalls[0].ID != "call_1" ||
		result.ToolCalls[0].Name != "project.get_overview" {
		t.Fatalf("unexpected tool call: %+v", result.ToolCalls[0])
	}
}

func TestChatCompletionAndPayload(t *testing.T) {
	t.Parallel()
	provider := newTestProvider(t, assistant.DialectChatCompletions, func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", request.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload["stream"] != true ||
			!strings.Contains(request.Header.Get("Accept"), "text/event-stream") {
			t.Errorf("chat completion is not streamed: %+v", payload)
		}
		messages := payload["messages"].([]any)
		if len(messages) != 2 || messages[0].(map[string]any)["role"] != "system" {
			t.Errorf("unexpected messages: %+v", messages)
		}
		tool := payload["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)
		if tool["strict"] != true {
			t.Errorf("tool is not strict: %+v", tool)
		}
		choice := payload["tool_choice"].(map[string]any)
		forced := choice["function"].(map[string]any)
		if choice["type"] != "function" || forced["name"] != "project_get_overview" {
			t.Errorf("forced Chat tool choice is missing: %+v", choice)
		}
		_, _ = io.WriteString(response, `{
			"id":"chat_1",
			"choices":[{"message":{
				"role":"assistant",
				"content":[{"type":"text","text":"Читаю."}],
				"reasoning_details":[{"type":"reasoning.encrypted","data":"opaque-state"}],
				"tool_calls":[{"id":"call_2","type":"function","function":{
					"name":"project_get_overview","arguments":"{}"
				}}]
			}}]
		}`)
	})
	result, err := provider.Complete(context.Background(), Request{
		Instructions: "System", Input: []json.RawMessage{UserInput("Что в проекте?")},
		Tools: []ToolDefinition{strictTestTool()}, ToolChoice: "project.get_overview",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "Читаю." || len(result.ToolCalls) != 1 ||
		result.ToolCalls[0].Name != "project.get_overview" ||
		len(result.Continuation) != 1 {
		t.Fatalf("unexpected completion: %+v", result)
	}
	if !strings.Contains(string(result.Continuation[0]), `"reasoning_details"`) ||
		!strings.Contains(string(result.Continuation[0]), `"opaque-state"`) {
		t.Fatalf("reasoning continuation was not preserved: %s", result.Continuation[0])
	}
}

func TestChatStreamingCompletionAssemblesToolCallAndOpaqueReasoning(t *testing.T) {
	t.Parallel()
	provider := newTestProvider(t, assistant.DialectChatCompletions, func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload["stream"] != true {
			t.Errorf("stream flag missing: %+v", payload)
		}
		response.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		_, _ = io.WriteString(response, ": OPENROUTER PROCESSING\n\n")
		_, _ = io.WriteString(response, "data: {\"id\":\"chat_stream_1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"reasoning_details\":[{\"type\":\"reasoning.encrypted\",\"data\":\"opaque-\",\"index\":0}]},\"finish_reason\":null}]}\n\n")
		_, _ = io.WriteString(response, "data: {\"id\":\"chat_stream_1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Чита\",\"reasoning_details\":[{\"type\":\"reasoning.encrypted\",\"data\":\"state\",\"index\":0}],\"tool_calls\":[{\"index\":0,\"id\":\"call_stream_1\",\"type\":\"function\",\"function\":{\"name\":\"project_get_\",\"arguments\":\"{\"}}]},\"finish_reason\":null}]}\n\n")
		_, _ = io.WriteString(response, "data: {\"id\":\"chat_stream_1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ю.\",\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"overview\",\"arguments\":\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n")
		_, _ = io.WriteString(response, "data: [DONE]\n\n")
	})

	result, err := provider.Complete(context.Background(), Request{
		Instructions: "System", Input: []json.RawMessage{UserInput("Что в проекте?")},
		Tools: []ToolDefinition{strictTestTool()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ResponseID != "chat_stream_1" || result.Text != "Читаю." ||
		len(result.ToolCalls) != 1 || result.ToolCalls[0].ID != "call_stream_1" ||
		result.ToolCalls[0].Name != "project.get_overview" ||
		string(result.ToolCalls[0].Arguments) != "{}" {
		t.Fatalf("stream completion was assembled incorrectly: %+v", result)
	}
	if len(result.Continuation) != 1 ||
		!strings.Contains(string(result.Continuation[0]), `"data":"opaque-state"`) ||
		!strings.Contains(string(result.Continuation[0]), `"tool_calls"`) {
		t.Fatalf("stream continuation lost protocol state: %s", result.Continuation[0])
	}
}

func TestChatStreamingCompletionReturnsSafeMidStreamError(t *testing.T) {
	t.Parallel()
	provider := newTestProvider(t, assistant.DialectChatCompletions, func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		response.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(response, "data: {\"id\":\"chat_error\",\"error\":{\"code\":524,\"message\":\"upstream timed out with private details\",\"metadata\":{\"error_type\":\"provider_timeout\",\"provider_name\":\"Anthropic\"}},\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"error\"}]}\n\n")
		_, _ = io.WriteString(response, "data: [DONE]\n\n")
	})

	_, err := provider.Complete(context.Background(), Request{})
	var providerError *ProviderHTTPError
	if !errors.As(err, &providerError) || providerError.StatusCode != 524 ||
		providerError.ProviderCode != "524" ||
		providerError.ErrorType != "provider_timeout" ||
		providerError.ProviderName != "Anthropic" {
		t.Fatalf("stream error diagnostics were lost: %T %+v", err, providerError)
	}
}

func TestChatStreamingCompletionRejectsUnterminatedStream(t *testing.T) {
	t.Parallel()
	provider := newTestProvider(t, assistant.DialectChatCompletions, func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		response.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(response, "data: {\"id\":\"chat_partial\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n")
	})
	if _, err := provider.Complete(context.Background(), Request{}); !errors.Is(
		err, ErrProviderResponse,
	) {
		t.Fatalf("unterminated stream was accepted: %v", err)
	}
}

func TestOpenAIWireToolNamesAreSafeAndUnambiguous(t *testing.T) {
	t.Parallel()
	wire, internalNames, err := prepareOpenAIWireRequest(Request{
		Tools: []ToolDefinition{
			{Name: "assistant.begin_turn"},
			{Name: "project_memory.add"},
		},
		ToolChoice: "assistant.begin_turn",
	})
	if err != nil {
		t.Fatal(err)
	}
	if wire.Tools[0].Name != "assistant_begin_turn" ||
		wire.Tools[1].Name != "project_memory_add" ||
		wire.ToolChoice != "assistant_begin_turn" ||
		internalNames["project_memory_add"] != "project_memory.add" {
		t.Fatalf("unexpected wire mapping: %+v %+v", wire, internalNames)
	}
	for _, request := range []Request{
		{Tools: []ToolDefinition{{Name: "a.b"}, {Name: "a_b"}}},
		{Tools: []ToolDefinition{{Name: "bad name"}}},
		{Tools: []ToolDefinition{{Name: "ok"}}, ToolChoice: "missing"},
	} {
		if _, _, err := prepareOpenAIWireRequest(request); err == nil {
			t.Fatalf("unsafe mapping was accepted: %+v", request)
		}
	}
}

func TestToolOutputShapes(t *testing.T) {
	t.Parallel()
	responses, err := ToolOutput(
		assistant.DialectResponses,
		"call_1",
		map[string]bool{"ok": true},
	)
	if err != nil || !strings.Contains(string(responses), "function_call_output") {
		t.Fatalf("unexpected Responses output: %s, %v", responses, err)
	}
	chat, err := ToolOutput(
		assistant.DialectChatCompletions,
		"call_1",
		map[string]bool{"ok": true},
	)
	if err != nil || !strings.Contains(string(chat), "tool_call_id") {
		t.Fatalf("unexpected chat output: %s, %v", chat, err)
	}
	if _, err := ToolOutput(assistant.DialectAuto, "call_1", true); err == nil {
		t.Fatal("unresolved dialect was accepted")
	}
	if _, err := ToolOutput(
		assistant.DialectResponses,
		"call_1",
		make(chan int),
	); err == nil {
		t.Fatal("unencodable tool output was accepted")
	}
}

func TestCompactToolCallArgumentsPreservesProtocolAndOpaqueState(t *testing.T) {
	t.Parallel()
	replacement := json.RawMessage(`{"nodeId":"node-1","baseRevision":1,"sequence":1,"targetWords":10000,"finalChunk":false,"text":"[omitted]"}`)
	tests := []struct {
		name         string
		dialect      assistant.ProviderDialect
		continuation []json.RawMessage
	}{
		{
			name:    "chat",
			dialect: assistant.DialectChatCompletions,
			continuation: []json.RawMessage{json.RawMessage(`{
				"role":"assistant","reasoning_details":[{"data":"opaque"}],
				"tool_calls":[{"id":"chunk-1","type":"function","function":{
					"name":"document_append_chunk","arguments":"{\"text\":\"expensive prose\"}"
				}}]
			}`)},
		},
		{
			name:    "responses",
			dialect: assistant.DialectResponses,
			continuation: []json.RawMessage{
				json.RawMessage(`{"type":"reasoning","id":"opaque"}`),
				json.RawMessage(`{"type":"function_call","call_id":"chunk-1","name":"document_append_chunk","arguments":"{\"text\":\"expensive prose\"}"}`),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compacted, err := CompactToolCallArguments(
				test.dialect,
				test.continuation,
				map[string]json.RawMessage{"chunk-1": replacement},
			)
			if err != nil {
				t.Fatal(err)
			}
			encoded := string(mustJoinJSON(compacted))
			if strings.Contains(encoded, "expensive prose") ||
				!strings.Contains(encoded, "[omitted]") ||
				!strings.Contains(encoded, "opaque") {
				t.Fatalf("continuation was not compacted safely: %s", encoded)
			}
		})
	}
	if _, err := CompactToolCallArguments(
		assistant.DialectAuto,
		[]json.RawMessage{json.RawMessage(`{}`)},
		map[string]json.RawMessage{"chunk-1": replacement},
	); err == nil {
		t.Fatal("unresolved dialect was accepted for continuation compaction")
	}
}

func mustJoinJSON(items []json.RawMessage) []byte {
	encoded, _ := json.Marshal(items)
	return encoded
}

func TestProviderHTTPErrorPreservesSafeDiagnostics(t *testing.T) {
	t.Parallel()
	provider := newTestProvider(t, assistant.DialectChatCompletions, func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		response.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(response, `{"error":{"code":"invalid_tools","message":"Too many tools\nfor this model","metadata":{"error_type":"invalid_request","provider_code":"schema_error","provider_name":"Azure"}}}`)
	})

	_, err := provider.Complete(context.Background(), Request{})
	var providerError *ProviderHTTPError
	if !errors.As(err, &providerError) || !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("provider HTTP error lost its error class: %T %v", err, err)
	}
	if providerError.StatusCode != http.StatusBadRequest ||
		providerError.ProviderCode != "schema_error" ||
		providerError.ErrorType != "invalid_request" ||
		providerError.ProviderName != "Azure" ||
		providerError.Message != "Too many tools for this model" {
		t.Fatalf("provider diagnostics were not normalized: %+v", providerError)
	}
}

func TestProviderTimeoutPreservesOnlySafeErrorClass(t *testing.T) {
	t.Parallel()
	baseURL, _ := url.Parse("https://secret-provider.example/v1")
	provider, err := NewOpenAICompatible(OpenAICompatibleConfig{
		BaseURL: baseURL, APIKey: "secret", Model: "writer",
		Dialect: assistant.DialectChatCompletions,
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Complete(context.Background(), Request{})
	if !errors.Is(err, ErrProviderUnavailable) ||
		!errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(err.Error(), "secret-provider") {
		t.Fatalf("provider timeout lost its safe class: %T %v", err, err)
	}
}

func TestChatStreamReadTimeoutPreservesSafeErrorClass(t *testing.T) {
	t.Parallel()
	_, err := parseChatCompletionStream(errorReader{err: context.DeadlineExceeded})
	if !errors.Is(err, ErrProviderUnavailable) ||
		!errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stream timeout lost its safe class: %T %v", err, err)
	}
}

type errorReader struct {
	err error
}

func (reader errorReader) Read([]byte) (int, error) {
	return 0, reader.err
}

func TestProviderConfigurationValidation(t *testing.T) {
	t.Parallel()
	validURL, _ := url.Parse("https://provider.example/v1/")
	if _, err := NewOpenAICompatible(OpenAICompatibleConfig{}); err == nil {
		t.Fatal("incomplete configuration accepted")
	}
	if _, err := NewOpenAICompatible(OpenAICompatibleConfig{
		BaseURL: validURL, APIKey: "key", Model: "model",
		Dialect: assistant.DialectAuto, HTTPClient: http.DefaultClient,
	}); err == nil {
		t.Fatal("unresolved dialect accepted")
	}
	for _, headers := range []map[string]string{
		{"Authorization": "other"},
		{"Host": "evil"},
		{"X-Bad": "line\nbreak"},
	} {
		if _, err := NewOpenAICompatible(OpenAICompatibleConfig{
			BaseURL: validURL, APIKey: "key", Model: "model",
			Dialect: assistant.DialectResponses, HTTPClient: http.DefaultClient,
			Headers: headers,
		}); err == nil {
			t.Fatalf("forbidden headers accepted: %+v", headers)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestProviderErrorsAreSanitized(t *testing.T) {
	t.Parallel()
	provider := newTestProvider(t, assistant.DialectResponses, func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		response.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(response, `{"error":"secret upstream detail"}`)
	})
	_, err := provider.Complete(context.Background(), Request{})
	if !errors.Is(err, ErrProviderUnavailable) ||
		strings.Contains(err.Error(), "secret upstream detail") {
		t.Fatalf("unsafe provider error: %v", err)
	}
}

func TestResponseParsersRejectMalformedData(t *testing.T) {
	t.Parallel()
	badResponses := []string{
		`not json`,
		`{"id":"","output":[]}`,
		`{"id":"x","output":[null]}`,
		`{"id":"x","output":[{"type":"function_call","call_id":"","name":"x","arguments":"{}"}]}`,
		`{"id":"x","output":[{"type":"function_call","call_id":"c","name":"x","arguments":"bad"}]}`,
	}
	for _, value := range badResponses {
		if _, err := parseResponsesCompletion([]byte(value)); !errors.Is(
			err,
			ErrProviderResponse,
		) {
			t.Fatalf("bad Responses data accepted: %s, %v", value, err)
		}
	}
	badChat := []string{
		`not json`,
		`{"id":"","choices":[]}`,
		`{"id":"x","choices":[]}`,
		`{"id":"x","choices":[{"message":null}]}`,
		`{"id":"x","choices":[{"message":{"tool_calls":[{"id":"","type":"function","function":{"name":"x","arguments":"{}"}}]}}]}`,
		`{"id":"x","choices":[{"message":{"tool_calls":[{"id":"c","type":"other","function":{"name":"x","arguments":"{}"}}]}}]}`,
		`{"id":"x","choices":[{"message":{"tool_calls":[{"id":"c","type":"function","function":{"name":"x","arguments":"bad"}}]}}]}`,
	}
	for _, value := range badChat {
		if _, err := parseChatCompletion([]byte(value)); !errors.Is(
			err,
			ErrProviderResponse,
		) {
			t.Fatalf("bad chat data accepted: %s, %v", value, err)
		}
	}
}

func TestChatContentText(t *testing.T) {
	t.Parallel()
	if chatContentText("text") != "text" {
		t.Fatal("string content lost")
	}
	parts := []any{
		map[string]any{"type": "text", "text": "one"},
		map[string]any{"type": "image", "url": "ignored"},
		"ignored",
		map[string]any{"type": "text", "text": "two"},
	}
	if chatContentText(parts) != "one\ntwo" || chatContentText(nil) != "" {
		t.Fatal("multipart content parsed incorrectly")
	}
}

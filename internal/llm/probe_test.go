package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"writers.studio/internal/assistant"
)

type scriptedProvider struct {
	complete func(context.Context, Request) (Completion, error)
}

func (p scriptedProvider) Complete(ctx context.Context, request Request) (Completion, error) {
	if p.complete == nil {
		return Completion{}, errors.New("scripted provider has no implementation")
	}
	result, err := p.complete(ctx, request)
	if err != nil {
		return Completion{}, fmt.Errorf("scripted provider: %w", err)
	}
	return result, nil
}

func TestProbeCompatibleToolFlow(t *testing.T) {
	t.Parallel()
	step := 0
	provider := scriptedProvider{complete: func(
		_ context.Context,
		request Request,
	) (Completion, error) {
		if len(request.Tools) != 2 ||
			request.Tools[0].Parameters["additionalProperties"] != false {
			t.Fatal("probe did not use strict tools")
		}
		step++
		switch step {
		case 1:
			return Completion{
				ResponseID: "one",
				ToolCalls: []ToolCall{{
					ID: "call_one", Name: "probe_echo", Arguments: json.RawMessage(`{"value":"alpha"}`),
				}},
				Continuation: []json.RawMessage{json.RawMessage(`{"type":"function_call"}`)},
			}, nil
		case 2:
			if len(request.Input) != 3 {
				t.Fatalf("first tool result missing: %d items", len(request.Input))
			}
			return Completion{
				ResponseID: "two",
				ToolCalls: []ToolCall{{
					ID: "call_two", Name: "probe_finish",
					Arguments: json.RawMessage(`{"previous":"alpha"}`),
				}},
				Continuation: []json.RawMessage{json.RawMessage(`{"type":"function_call"}`)},
			}, nil
		case 3:
			if len(request.Input) != 5 {
				t.Fatalf("second tool result missing: %d items", len(request.Input))
			}
			return Completion{ResponseID: "three", Text: "OK"}, nil
		default:
			t.Fatal("unexpected probe call")
		}
		return Completion{}, nil
	}}
	report := Probe(
		context.Background(),
		provider,
		assistant.DialectResponses,
		"writer",
	)
	if report.Status != "limited" || !report.Capabilities.CanRunAssistant() ||
		len(report.Capabilities.Limitations) != 1 || step != 3 {
		t.Fatalf("unexpected probe report: %+v", report)
	}
}

func TestProbeFailureModes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		provider Provider
		dialect  assistant.ProviderDialect
	}{
		{name: "missing provider", provider: nil, dialect: assistant.DialectResponses},
		{
			name: "first request failed",
			provider: scriptedProvider{complete: func(
				context.Context,
				Request,
			) (Completion, error) {
				return Completion{}, errors.New("offline")
			}},
			dialect: assistant.DialectResponses,
		},
		{
			name: "wrong first tool",
			provider: sequenceProvider(Completion{
				ResponseID: "one",
				ToolCalls: []ToolCall{{
					ID: "call", Name: "wrong", Arguments: json.RawMessage(`{}`),
				}},
			}),
			dialect: assistant.DialectResponses,
		},
		{
			name: "wrong first arguments",
			provider: sequenceProvider(Completion{
				ResponseID: "one",
				ToolCalls: []ToolCall{{
					ID: "call", Name: "probe_echo",
					Arguments: json.RawMessage(`{"value":"beta"}`),
				}},
			}),
			dialect: assistant.DialectResponses,
		},
		{
			name: "unsupported dialect output",
			provider: sequenceProvider(Completion{
				ResponseID: "one",
				ToolCalls: []ToolCall{{
					ID: "call", Name: "probe_echo",
					Arguments: json.RawMessage(`{"value":"alpha"}`),
				}},
			}),
			dialect: assistant.DialectAuto,
		},
		{
			name: "second request failed",
			provider: sequenceProvider(
				Completion{
					ResponseID: "one",
					ToolCalls: []ToolCall{{
						ID: "call", Name: "probe_echo",
						Arguments: json.RawMessage(`{"value":"alpha"}`),
					}},
				},
				errors.New("offline"),
			),
			dialect: assistant.DialectResponses,
		},
		{
			name: "wrong second tool",
			provider: sequenceProvider(
				Completion{
					ResponseID: "one",
					ToolCalls: []ToolCall{{
						ID: "call", Name: "probe_echo",
						Arguments: json.RawMessage(`{"value":"alpha"}`),
					}},
				},
				Completion{
					ResponseID: "two",
					ToolCalls: []ToolCall{{
						ID: "call2", Name: "wrong", Arguments: json.RawMessage(`{}`),
					}},
				},
			),
			dialect: assistant.DialectResponses,
		},
		{
			name: "second loses state",
			provider: sequenceProvider(
				Completion{
					ResponseID: "one",
					ToolCalls: []ToolCall{{
						ID: "call", Name: "probe_echo",
						Arguments: json.RawMessage(`{"value":"alpha"}`),
					}},
				},
				Completion{
					ResponseID: "two",
					ToolCalls: []ToolCall{{
						ID: "call2", Name: "probe_finish",
						Arguments: json.RawMessage(`{"previous":"beta"}`),
					}},
				},
			),
			dialect: assistant.DialectResponses,
		},
		{
			name: "final request has no text",
			provider: sequenceProvider(
				Completion{
					ResponseID: "one",
					ToolCalls: []ToolCall{{
						ID: "call", Name: "probe_echo",
						Arguments: json.RawMessage(`{"value":"alpha"}`),
					}},
				},
				Completion{
					ResponseID: "two",
					ToolCalls: []ToolCall{{
						ID: "call2", Name: "probe_finish",
						Arguments: json.RawMessage(`{"previous":"alpha"}`),
					}},
				},
				Completion{ResponseID: "three"},
			),
			dialect: assistant.DialectResponses,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			report := Probe(context.Background(), test.provider, test.dialect, "writer")
			if report.Status != "unavailable" || report.Detail == "" {
				t.Fatalf("unexpected report: %+v", report)
			}
		})
	}
}

func sequenceProvider(values ...any) Provider {
	index := 0
	return scriptedProvider{complete: func(
		context.Context,
		Request,
	) (Completion, error) {
		if index >= len(values) {
			return Completion{}, errors.New("sequence exhausted")
		}
		value := values[index]
		index++
		switch typed := value.(type) {
		case Completion:
			return typed, nil
		case error:
			return Completion{}, typed
		default:
			return Completion{}, errors.New("invalid sequence value")
		}
	}}
}

func TestFailedProbeAndScriptedProviderDefaults(t *testing.T) {
	t.Parallel()
	report := failedProbe(assistant.CapabilitySnapshot{}, "")
	if report.Status != "unavailable" || report.Detail == "" {
		t.Fatalf("unexpected default failure: %+v", report)
	}
	var provider scriptedProvider
	if _, err := provider.Complete(context.Background(), Request{}); err == nil {
		t.Fatal("empty scripted provider succeeded")
	}
}

package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

// FixtureProvider is a deterministic, in-process provider for browser and
// integration tests. Configuration rejects it in production, so the runtime
// cannot silently substitute it for an author's real model.
type FixtureProvider struct {
	sequence atomic.Uint64
	mu       sync.Mutex
	scenario *fixtureScenario
	stage    int
}

type fixtureScenario struct {
	Fixture     string `json:"fixture"`
	NodeID      string `json:"nodeId"`
	Revision    int    `json:"revision"`
	Expected    string `json:"expected"`
	Replacement string `json:"replacement"`
}

func NewFixtureProvider() *FixtureProvider {
	return &FixtureProvider{}
}

func (p *FixtureProvider) Complete(ctx context.Context, request Request) (Completion, error) {
	if err := ctx.Err(); err != nil {
		return Completion{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	responseID := fmt.Sprintf("fixture-response-%d", p.sequence.Add(1))
	toolCall := func(name string, arguments any) (Completion, error) {
		if !requestHasTool(request, name) {
			return Completion{}, fmt.Errorf("fixture request does not declare tool %q", name)
		}
		encoded, err := json.Marshal(arguments)
		if err != nil {
			return Completion{}, err
		}
		return Completion{
			ResponseID: responseID,
			ToolCalls: []ToolCall{{
				ID: responseID + "-call", Name: name, Arguments: encoded,
			}},
		}, nil
	}

	switch request.ToolChoice {
	case "probe_echo":
		return toolCall("probe_echo", map[string]string{"value": "alpha"})
	case "probe_finish":
		return toolCall("probe_finish", map[string]string{"previous": "alpha"})
	case "assistant.begin_turn":
		p.scenario = fixtureScenarioFromInput(request.Input)
		p.stage = 0
		mode := "answer"
		summary := "Детерминированный тестовый ответ"
		if p.scenario != nil {
			mode = "change"
			summary = "Подготовить точную тестовую замену"
		}
		return toolCall("assistant.begin_turn", map[string]string{
			"mode": mode, "summary": summary,
		})
	case "":
		return p.completeScenario(request, responseID, toolCall)
	default:
		return Completion{}, fmt.Errorf("fixture tool choice %q is unsupported", request.ToolChoice)
	}
}

func (p *FixtureProvider) completeScenario(
	request Request,
	responseID string,
	toolCall func(string, any) (Completion, error),
) (Completion, error) {
	if p.scenario == nil {
		return Completion{ResponseID: responseID, Text: "Готово."}, nil
	}
	switch p.stage {
	case 0:
		p.stage = 1
		return toolCall("document.find_exact", map[string]any{
			"nodeId": p.scenario.NodeID, "revision": p.scenario.Revision,
			"text": p.scenario.Expected, "blockId": nil,
		})
	case 1:
		output := fixtureToolOutput(request.Input)
		matches, _ := output["matches"].([]any)
		if len(matches) != 1 {
			return Completion{}, fmt.Errorf("fixture expected one exact match, got %d", len(matches))
		}
		match, ok := matches[0].(map[string]any)
		if !ok {
			return Completion{}, fmt.Errorf("fixture exact match has an invalid shape")
		}
		p.stage = 2
		return toolCall("document.replace_range", map[string]any{
			"nodeId": p.scenario.NodeID, "baseRevision": p.scenario.Revision,
			"edits": []any{map[string]any{
				"blockId": match["blockId"], "start": match["start"], "end": match["end"],
				"expectedText": p.scenario.Expected, "replacement": p.scenario.Replacement,
			}},
			"protectedRanges": []any{},
		})
	default:
		return Completion{
			ResponseID: responseID,
			Text:       "## Правка готова\n\nПроверьте точную замену перед применением.",
		}, nil
	}
}

func fixtureScenarioFromInput(input []json.RawMessage) *fixtureScenario {
	for index := len(input) - 1; index >= 0; index-- {
		var message struct {
			Content string `json:"content"`
		}
		if json.Unmarshal(input[index], &message) != nil || message.Content == "" {
			continue
		}
		var scenario fixtureScenario
		if json.Unmarshal([]byte(message.Content), &scenario) != nil ||
			scenario.Fixture != "exact_replace" || scenario.NodeID == "" ||
			scenario.Revision < 1 || scenario.Expected == "" {
			continue
		}
		return &scenario
	}
	return nil
}

func fixtureToolOutput(input []json.RawMessage) map[string]any {
	for index := len(input) - 1; index >= 0; index-- {
		var item struct {
			Role    string `json:"role"`
			Content string `json:"content"`
			Type    string `json:"type"`
			Output  string `json:"output"`
		}
		if json.Unmarshal(input[index], &item) != nil {
			continue
		}
		encoded := ""
		if item.Role == "tool" {
			encoded = item.Content
		} else if item.Type == "function_call_output" {
			encoded = item.Output
		}
		var output map[string]any
		if encoded != "" && json.Unmarshal([]byte(encoded), &output) == nil {
			return output
		}
	}
	return nil
}

func requestHasTool(request Request, name string) bool {
	for _, tool := range request.Tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

package llm

import (
	"context"
	"encoding/json"
	"testing"

	"writers.studio/internal/assistant"
)

func TestFixtureProviderPassesStrictProbe(t *testing.T) {
	t.Parallel()
	report := Probe(
		context.Background(),
		NewFixtureProvider(),
		assistant.DialectChatCompletions,
		"fixture",
	)
	if report.Status != "limited" || !report.Capabilities.CanRunAssistant() {
		t.Fatalf("fixture provider failed strict probe: %+v", report)
	}
}

func TestFixtureProviderStagesExactReplacementThroughVerifiedAnchors(t *testing.T) {
	t.Parallel()
	provider := NewFixtureProvider()
	scenario, _ := json.Marshal(fixtureScenario{
		Fixture: "exact_replace", NodeID: "node-1", Revision: 2,
		Expected: "старый свет", Replacement: "новый свет",
	})
	declaration, err := provider.Complete(context.Background(), Request{
		Input:      []json.RawMessage{UserInput(string(scenario))},
		Tools:      []ToolDefinition{{Name: "assistant.begin_turn"}},
		ToolChoice: "assistant.begin_turn",
	})
	if err != nil || string(declaration.ToolCalls[0].Arguments) == "" {
		t.Fatalf("fixture change declaration failed: %+v, %v", declaration, err)
	}
	find, err := provider.Complete(context.Background(), Request{
		Tools: []ToolDefinition{{Name: "document.find_exact"}},
	})
	if err != nil || len(find.ToolCalls) != 1 || find.ToolCalls[0].Name != "document.find_exact" {
		t.Fatalf("fixture exact lookup failed: %+v, %v", find, err)
	}
	output, err := ToolOutput(
		assistant.DialectChatCompletions,
		find.ToolCalls[0].ID,
		map[string]any{"matches": []any{map[string]any{
			"blockId": "block-1", "start": 3, "end": 14, "expectedText": "старый свет",
		}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	replace, err := provider.Complete(context.Background(), Request{
		Input: []json.RawMessage{output},
		Tools: []ToolDefinition{{Name: "document.replace_range"}},
	})
	if err != nil || len(replace.ToolCalls) != 1 ||
		replace.ToolCalls[0].Name != "document.replace_range" {
		t.Fatalf("fixture exact replacement failed: %+v, %v", replace, err)
	}
	var arguments struct {
		Edits []struct {
			BlockID     string `json:"blockId"`
			Expected    string `json:"expectedText"`
			Replacement string `json:"replacement"`
		} `json:"edits"`
	}
	if json.Unmarshal(replace.ToolCalls[0].Arguments, &arguments) != nil ||
		len(arguments.Edits) != 1 || arguments.Edits[0].BlockID != "block-1" ||
		arguments.Edits[0].Expected != "старый свет" ||
		arguments.Edits[0].Replacement != "новый свет" {
		t.Fatalf("fixture replacement arguments drifted: %s", replace.ToolCalls[0].Arguments)
	}
}

func TestFixtureProviderDeclaresAnAnswerTurn(t *testing.T) {
	t.Parallel()
	provider := NewFixtureProvider()
	completion, err := provider.Complete(context.Background(), Request{
		Tools:      []ToolDefinition{{Name: "assistant.begin_turn"}},
		ToolChoice: "assistant.begin_turn",
	})
	if err != nil || len(completion.ToolCalls) != 1 ||
		completion.ToolCalls[0].Name != "assistant.begin_turn" {
		t.Fatalf("fixture declaration failed: %+v, %v", completion, err)
	}
	terminal, err := provider.Complete(context.Background(), Request{})
	if err != nil || terminal.Text != "Готово." || len(terminal.ToolCalls) != 0 {
		t.Fatalf("fixture terminal response failed: %+v, %v", terminal, err)
	}
}

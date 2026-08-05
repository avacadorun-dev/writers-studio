package store

import (
	"encoding/json"
	"testing"

	"writers.studio/internal/assistant"
)

func TestSelectAssistantGroups(t *testing.T) {
	t.Parallel()
	operations := []assistant.ChangeOperation{
		{GroupKey: "document", Selected: true},
		{GroupKey: "dictionary", Selected: false},
		{
			GroupKey: "memory", Selected: false,
			Dependencies: []string{"document"},
		},
	}
	selected, err := selectAssistantGroups(operations, nil)
	if err != nil || !selected["document"] || selected["dictionary"] {
		t.Fatalf("default selection failed: %+v, %v", selected, err)
	}
	selected, err = selectAssistantGroups(operations, []string{"document", "memory"})
	if err != nil || !selected["memory"] {
		t.Fatalf("explicit dependency selection failed: %+v, %v", selected, err)
	}
	if _, err := selectAssistantGroups(operations, []string{"memory"}); err == nil {
		t.Fatal("missing group dependency was accepted")
	}
	if _, err := selectAssistantGroups(operations, []string{"unknown"}); err == nil {
		t.Fatal("unknown group was accepted")
	}
	for index := range operations {
		operations[index].Selected = false
	}
	if _, err := selectAssistantGroups(operations, nil); err == nil {
		t.Fatal("empty group selection was accepted")
	}
}

func TestAssistantAnnotationEditsAndResolutionOverrides(t *testing.T) {
	t.Parallel()
	arguments, _ := json.Marshal(map[string]any{
		"edits": []map[string]any{{
			"blockId": "block", "start": 1, "end": 4,
			"expectedText": "old", "replacement": "new",
		}},
		"annotationFixes": []map[string]any{{
			"annotationId": "note", "annotationRevision": 3,
			"originalText": "old", "resolve": true,
		}},
	})
	operation := assistant.ChangeOperation{
		GroupKey: "annotation:node", ToolName: "annotation.propose_fixes",
		Arguments: arguments,
	}
	edits, resolutions, expectations, err := assistantAnnotationEdits(operation)
	if err != nil || len(edits) != 1 || edits[0].Expected != "old" ||
		!resolutions["note"] || expectations["note"].Revision != 3 {
		t.Fatalf("annotation edit metadata = %#v %#v %#v, err=%v", edits, resolutions, expectations, err)
	}
	selected := map[string]bool{"annotation:node": true}
	if err := validateAnnotationResolutionOverrides(
		[]assistant.ChangeOperation{operation}, selected, map[string]bool{"note": false},
	); err != nil {
		t.Fatalf("valid resolution override rejected: %v", err)
	}
	if err := validateAnnotationResolutionOverrides(
		[]assistant.ChangeOperation{operation}, selected, map[string]bool{"other": true},
	); err == nil {
		t.Fatal("unknown resolution override accepted")
	}
	if err := validateAnnotationResolutionOverrides(
		[]assistant.ChangeOperation{operation}, map[string]bool{}, map[string]bool{"note": true},
	); err == nil {
		t.Fatal("override for an unselected operation accepted")
	}
}

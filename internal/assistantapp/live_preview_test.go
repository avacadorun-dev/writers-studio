package assistantapp

import (
	"encoding/json"
	"testing"

	"writers.studio/internal/assistant"
)

func TestPendingDocumentPreviewUsesStagedProposal(t *testing.T) {
	t.Parallel()
	targetID := "node"
	baseRevision := 5
	backend := &ProjectBackend{pending: map[string][]assistant.ChangeOperation{
		"turn": {{
			ID: "operation", TargetKind: "document", TargetID: &targetID,
			BaseRevision: &baseRevision, ToolName: "document.replace_range",
			Arguments:        json.RawMessage(`{"edits":[]}`),
			ProposedSnapshot: json.RawMessage(`{"type":"doc","content":[]}`),
		}},
	}}
	preview, ok := backend.PendingDocumentPreview("turn")
	if !ok || preview["operationId"] != "operation" || preview["targetNodeId"] != targetID ||
		preview["canonical"] != false {
		t.Fatalf("unexpected live preview: %#v", preview)
	}
	if _, ok := backend.PendingDocumentPreview("missing"); ok {
		t.Fatal("missing turn exposed a preview")
	}
}

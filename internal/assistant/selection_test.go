package assistant

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestValidateSelectionAttachmentUsesStableAnchors(t *testing.T) {
	t.Parallel()
	document := json.RawMessage(`{
		"type":"doc",
		"content":[
			{"type":"paragraph","attrs":{"blockId":"first"},"content":[{"type":"text","text":"Первый абзац."}]},
			{"type":"paragraph","attrs":{"blockId":"second"},"content":[{"type":"text","text":"Нужный точный фрагмент."}]}
		]
	}`)
	selection := SelectionAttachment{
		NodeID: "node", BaseRevision: 7, Quote: "точный фрагмент",
		Ranges: []SelectionRange{{
			BlockID: "second", Start: 7, End: 22, ExpectedText: "точный фрагмент",
		}},
	}
	if err := ValidateSelectionAttachment(document, selection); err != nil {
		t.Fatalf("valid stable selection was rejected: %v", err)
	}
	selection.Ranges[0].ExpectedText = "другой фрагмент"
	selection.Quote = "другой фрагмент"
	if err := ValidateSelectionAttachment(document, selection); !errors.Is(err, ErrReferenceNotFound) {
		t.Fatalf("mismatched selection was not rejected: %v", err)
	}
}

func TestSelectionAttachmentRejectsMalformedRanges(t *testing.T) {
	t.Parallel()
	selection := SelectionAttachment{
		NodeID: "node", BaseRevision: 1, Quote: "текст",
		Ranges: []SelectionRange{{
			BlockID: "block", Start: 4, End: 2, ExpectedText: "текст",
		}},
	}
	if err := selection.Validate(); err == nil {
		t.Fatal("backwards selection range was accepted")
	}
}

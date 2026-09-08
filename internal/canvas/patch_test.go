package canvas

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestApplyPatchAddsUpdatesDeletesAndPreservesExtensions(t *testing.T) {
	raw := json.RawMessage(`{"nodes":[{"id":"a","type":"text","x":0,"y":0,"width":100,"height":100,"text":"A","future":{"keep":true}},{"id":"b","type":"text","x":200,"y":0,"width":100,"height":100,"text":"B"}],"edges":[{"id":"ab","fromNode":"a","toNode":"b","future":5}],"future":{"root":true}}`)
	result, err := ApplyPatch(raw, []PatchOperation{
		{Op: "update", Kind: "node", ID: "a", Value: json.RawMessage(`{"id":"a","type":"text","x":20,"y":0,"width":100,"height":100,"text":"A2","future":{"keep":true}}`)},
		{Op: "add", Kind: "node", ID: "c", Value: json.RawMessage(`{"id":"c","type":"text","x":400,"y":0,"width":100,"height":100,"text":"C"}`)},
		{Op: "add", Kind: "edge", ID: "ac", Value: json.RawMessage(`{"id":"ac","fromNode":"a","toNode":"c"}`)},
		{Op: "delete", Kind: "node", ID: "b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(result)
	for _, expected := range []string{`"A2"`, `"id":"c"`, `"id":"ac"`, `"root":true`, `"keep":true`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("result %s misses %s", text, expected)
		}
	}
	if strings.Contains(text, `"id":"b"`) || strings.Contains(text, `"id":"ab"`) {
		t.Fatalf("deleted node or dangling edge remains: %s", text)
	}
}

func TestApplyPatchRejectsMalformedOperationsAndInvalidResults(t *testing.T) {
	raw := json.RawMessage(`{"nodes":[{"id":"a","type":"text","x":0,"y":0,"width":100,"height":100,"text":"A"}],"edges":[]}`)
	tests := [][]PatchOperation{
		nil,
		{{Op: "add", Kind: "shape", ID: "b", Value: json.RawMessage(`{"id":"b"}`)}},
		{{Op: "unknown", Kind: "node", ID: "a"}},
		{{Op: "add", Kind: "node", ID: "a", Value: json.RawMessage(`{"id":"a"}`)}},
		{{Op: "add", Kind: "node", ID: "b", Value: json.RawMessage(`{"id":"other"}`)}},
		{{Op: "update", Kind: "node", ID: "missing", Value: json.RawMessage(`{"id":"missing"}`)}},
		{{Op: "delete", Kind: "node", ID: "a", Value: json.RawMessage(`{}`)}},
		{{Op: "update", Kind: "node", ID: "a", Value: json.RawMessage(`{"id":"a","type":"text","x":0,"y":0,"width":10,"height":10,"text":"bad"}`)}},
		{{Op: "delete", Kind: "edge", ID: "missing"}},
		{{Op: "delete", Kind: "node", ID: "bad id"}},
	}
	for _, operations := range tests {
		if _, err := ApplyPatch(raw, operations); !errors.Is(err, ErrInvalidDocument) {
			t.Fatalf("ApplyPatch(%#v) error = %v", operations, err)
		}
	}
	tooMany := make([]PatchOperation, MaxPatchOperations+1)
	if _, err := ApplyPatch(raw, tooMany); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("operation limit error = %v", err)
	}
	if _, err := ApplyPatch(json.RawMessage(`not-json`), []PatchOperation{{Op: "delete", Kind: "node", ID: "a"}}); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("malformed canvas error = %v", err)
	}
}

package canvas

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestPromoteNodeReplacesIdeaWithoutChangingGraphIdentity(t *testing.T) {
	raw := json.RawMessage(`{
		"nodes":[{"id":"idea","type":"text","x":0,"y":0,"width":200,"height":120,"text":"# Квест\nНайти маяк","x-writers-studio":{"card":{"typeId":"quest","fields":{"reward":5}},"future":{"kept":true}}},{"id":"target","type":"text","x":300,"y":0,"width":200,"height":120,"text":"Финал"}],
		"edges":[{"id":"edge","fromNode":"idea","toNode":"target"}],
		"x-writers-studio":{"schemaVersion":2,"future":true}
	}`)
	result, err := PromoteNode(raw, "idea", "canvas", "document")
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceText != "# Квест\nНайти маяк" {
		t.Fatalf("SourceText = %q", result.SourceText)
	}
	var value map[string]any
	if err := json.Unmarshal(result.Content, &value); err != nil {
		t.Fatal(err)
	}
	nodes := value["nodes"].([]any)
	promoted := nodes[0].(map[string]any)
	if promoted["id"] != "idea" || promoted["type"] != "file" || promoted["text"] != nil {
		t.Fatalf("promoted node = %#v", promoted)
	}
	extension := promoted["x-writers-studio"].(map[string]any)
	if extension["future"].(map[string]any)["kept"] != true {
		t.Fatal("unknown extension was discarded")
	}
	card := extension["card"].(map[string]any)
	if card["promotedDocumentId"] != "document" {
		t.Fatalf("card metadata = %#v", card)
	}
	references, err := Validate(result.Content)
	if err != nil || len(references.DocumentIDs) != 1 || references.DocumentIDs[0] != "document" {
		t.Fatalf("references=%#v error=%v", references, err)
	}
}

func TestPromoteNodeSupportsLinkAndGroupAndRejectsInvalidSources(t *testing.T) {
	for _, node := range []string{
		`{"id":"n","type":"link","x":0,"y":0,"width":100,"height":100,"url":"https://example.com"}`,
		`{"id":"n","type":"group","x":0,"y":0,"width":100,"height":100,"label":"Группа"}`,
	} {
		raw := json.RawMessage(`{"nodes":[` + node + `],"edges":[]}`)
		if _, err := PromoteNode(raw, "n", "canvas", "document"); err != nil {
			t.Fatalf("PromoteNode(%s) error = %v", node, err)
		}
	}
	tests := []struct {
		raw, nodeID, canvasID string
	}{
		{`{"nodes":[],"edges":[]}`, "missing", "canvas"},
		{`{"nodes":[{"id":"n","type":"text","x":0,"y":0,"width":100,"height":100,"text":" "}],"edges":[]}`, "n", "canvas"},
		{`{"nodes":[{"id":"n","type":"file","x":0,"y":0,"width":100,"height":100,"file":"a"}],"edges":[]}`, "n", "canvas"},
		{`not-json`, "n", "canvas"},
		{`{"nodes":[],"edges":[]}`, "bad id", "canvas"},
	}
	for _, item := range tests {
		_, err := PromoteNode(json.RawMessage(item.raw), item.nodeID, item.canvasID, "document")
		if !errors.Is(err, ErrInvalidDocument) {
			t.Fatalf("PromoteNode(%q) error = %v", item.raw, err)
		}
	}
	brokenExtension := `{"nodes":[{"id":"n","type":"text","x":0,"y":0,"width":100,"height":100,"text":"ok","x-writers-studio":` + strings.Repeat("[", 2) + `}],"edges":[]}`
	if _, err := PromoteNode(json.RawMessage(brokenExtension), "n", "canvas", "document"); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("broken extension error = %v", err)
	}
}

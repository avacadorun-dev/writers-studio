package canvas

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
)

func TestValidateCanvasDocument(t *testing.T) {
	raw := json.RawMessage(`{
		"nodes":[
			{"id":"note","type":"text","x":0,"y":0,"width":320,"height":200,"text":"# Замысел"},
			{"id":"scene","type":"file","x":400,"y":0,"width":360,"height":240,"file":"documents/scene.md","x-writers-studio":{"resource":{"kind":"document","id":"doc-id"}}},
			{"id":"group","type":"group","x":-40,"y":-80,"width":840,"height":380,"label":"Акт I"}
		],
		"edges":[{"id":"edge","fromNode":"note","fromSide":"right","toNode":"scene","toSide":"left","toEnd":"arrow","label":"развить"}],
		"x-writers-studio":{"schemaVersion":1,"future":{"preserve":true}}
	}`)
	references, err := Validate(raw)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(references.DocumentIDs) != 1 || references.DocumentIDs[0] != "doc-id" {
		t.Fatalf("unexpected document refs: %#v", references.DocumentIDs)
	}
}

func TestValidateRejectsDanglingEdgeAndUnknownResource(t *testing.T) {
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{"nodes":[],"edges":[{"id":"e","fromNode":"missing","toNode":"missing"}]}`),
		json.RawMessage(`{"nodes":[{"id":"f","type":"file","x":0,"y":0,"width":100,"height":100,"file":"x","x-writers-studio":{"resource":{"kind":"audio","id":"a"}}}],"edges":[]}`),
	} {
		if _, err := Validate(raw); !errors.Is(err, ErrInvalidDocument) {
			t.Fatalf("expected invalid document, got %v", err)
		}
	}
}

func TestEmptyDocumentIsValid(t *testing.T) {
	references, err := Validate(EmptyDocument())
	if err != nil {
		t.Fatal(err)
	}
	if len(references.DocumentIDs) != 0 || len(references.MediaIDs) != 0 {
		t.Fatalf("unexpected references: %#v", references)
	}
}

func TestValidateCollectsSortedUniqueResourcesAndAllEdgeOptions(t *testing.T) {
	raw := json.RawMessage(`{
		"nodes":[
			{"id":"a","type":"file","x":0,"y":0,"width":100,"height":100,"color":"1","file":"a","x-writers-studio":{"resource":{"kind":"document","id":"z-doc"}}},
			{"id":"b","type":"file","x":120,"y":0,"width":100,"height":100,"color":"#abcdef88","file":"b","x-writers-studio":{"resource":{"kind":"document","id":"a-doc"}}},
			{"id":"c","type":"file","x":240,"y":0,"width":100,"height":100,"file":"c","x-writers-studio":{"resource":{"kind":"media","id":"media"}}}
		],
		"edges":[
			{"id":"one","fromNode":"a","fromSide":"top","fromEnd":"arrow","toNode":"b","toSide":"bottom","toEnd":"none","color":"#abcdef","label":"one"},
			{"id":"two","fromNode":"b","fromSide":"left","fromEnd":"none","toNode":"c","toSide":"right","toEnd":"arrow"}
		]
	}`)
	references, err := Validate(raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(references.DocumentIDs, ",") != "a-doc,z-doc" ||
		strings.Join(references.MediaIDs, ",") != "media" {
		t.Fatalf("unexpected sorted references: %#v", references)
	}
}

func TestValidateRejectsEveryInvalidShape(t *testing.T) {
	longText := strings.Repeat("x", MaxTextRunes+1)
	longLabel := strings.Repeat("x", 501)
	validText := `{"id":"a","type":"text","x":0,"y":0,"width":100,"height":100,"text":"ok"}`
	tests := map[string]string{
		"empty":               ``,
		"not json":            `{`,
		"not object":          `[]`,
		"bad shape":           `{"nodes":"bad","edges":[]}`,
		"missing id":          `{"nodes":[{"type":"text","x":0,"y":0,"width":100,"height":100,"text":"ok"}],"edges":[]}`,
		"duplicate node":      `{"nodes":[` + validText + `,` + validText + `],"edges":[]}`,
		"fractional geometry": `{"nodes":[{"id":"a","type":"text","x":0.5,"y":0,"width":100,"height":100,"text":"ok"}],"edges":[]}`,
		"small geometry":      `{"nodes":[{"id":"a","type":"text","x":0,"y":0,"width":39,"height":100,"text":"ok"}],"edges":[]}`,
		"bad node color":      `{"nodes":[{"id":"a","type":"text","x":0,"y":0,"width":100,"height":100,"color":"pink","text":"ok"}],"edges":[]}`,
		"missing text":        `{"nodes":[{"id":"a","type":"text","x":0,"y":0,"width":100,"height":100}],"edges":[]}`,
		"long text":           `{"nodes":[{"id":"a","type":"text","x":0,"y":0,"width":100,"height":100,"text":` + quoted(longText) + `}],"edges":[]}`,
		"missing file":        `{"nodes":[{"id":"a","type":"file","x":0,"y":0,"width":100,"height":100,"x-writers-studio":{"resource":{"kind":"document","id":"doc"}}}],"edges":[]}`,
		"missing extension":   `{"nodes":[{"id":"a","type":"file","x":0,"y":0,"width":100,"height":100,"file":"a"}],"edges":[]}`,
		"empty resource id":   `{"nodes":[{"id":"a","type":"file","x":0,"y":0,"width":100,"height":100,"file":"a","x-writers-studio":{"resource":{"kind":"document","id":""}}}],"edges":[]}`,
		"unknown resource":    `{"nodes":[{"id":"a","type":"file","x":0,"y":0,"width":100,"height":100,"file":"a","x-writers-studio":{"resource":{"kind":"web","id":"web"}}}],"edges":[]}`,
		"long group label":    `{"nodes":[{"id":"a","type":"group","x":0,"y":0,"width":100,"height":100,"label":` + quoted(longLabel) + `}],"edges":[]}`,
		"unknown node":        `{"nodes":[{"id":"a","type":"drawing","x":0,"y":0,"width":100,"height":100}],"edges":[]}`,
		"missing edge id":     `{"nodes":[` + validText + `],"edges":[{"fromNode":"a","toNode":"a"}]}`,
		"duplicate edge":      `{"nodes":[` + validText + `],"edges":[{"id":"e","fromNode":"a","toNode":"a"},{"id":"e","fromNode":"a","toNode":"a"}]}`,
		"missing edge target": `{"nodes":[` + validText + `],"edges":[{"id":"e","fromNode":"a","toNode":"b"}]}`,
		"bad edge side":       `{"nodes":[` + validText + `],"edges":[{"id":"e","fromNode":"a","fromSide":"center","toNode":"a"}]}`,
		"bad edge end":        `{"nodes":[` + validText + `],"edges":[{"id":"e","fromNode":"a","fromEnd":"circle","toNode":"a"}]}`,
		"bad edge color":      `{"nodes":[` + validText + `],"edges":[{"id":"e","fromNode":"a","toNode":"a","color":"blue"}]}`,
		"long edge label":     `{"nodes":[` + validText + `],"edges":[{"id":"e","fromNode":"a","toNode":"a","label":` + quoted(longLabel) + `}]}`,
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Validate(json.RawMessage(raw)); !errors.Is(err, ErrInvalidDocument) {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}

	tooManyNodes := document{Nodes: make([]node, MaxNodes+1), Edges: []edge{}}
	encoded, err := json.Marshal(tooManyNodes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Validate(encoded); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("oversized canvas error = %v", err)
	}
	if integerInRange(math.NaN(), 0, 1) || integerInRange(math.Inf(1), 0, 1) {
		t.Fatal("non-finite geometry was accepted")
	}
}

func quoted(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

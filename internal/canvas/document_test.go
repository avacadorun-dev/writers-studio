package canvas

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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

func TestValidateStandardJSONCanvasFileNodeWithoutProjectResource(t *testing.T) {
	references, err := Validate(json.RawMessage(`{
		"nodes":[{"id":"file","type":"file","x":0,"y":0,"width":240,"height":120,"file":"research/map.pdf","custom":{"preserved":true}}],
		"edges":[]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(references.DocumentIDs)+len(references.MediaIDs)+len(references.CanvasIDs) != 0 {
		t.Fatalf("unexpected external file references: %#v", references)
	}
}

func TestValidateCanvasDocumentV2ElementsLinksAndNestedCanvas(t *testing.T) {
	raw := json.RawMessage(`{
		"nodes":[
			{"id":"stroke","type":"group","x":0,"y":0,"width":100,"height":100,"x-writers-studio":{"element":{"kind":"stroke","points":[[5,5,0.2],[50,50,0.8]],"brush":"pen","color":"#112233","size":4,"opacity":1}}},
			{"id":"shape","type":"group","x":120,"y":0,"width":100,"height":100,"x-writers-studio":{"element":{"kind":"shape","shape":"diamond","color":"5","fill":"transparent","size":3,"opacity":0.8,"rotation":45,"dash":"dashed","label":"Choice"}}},
			{"id":"frame","type":"group","x":-20,"y":-40,"width":400,"height":240,"x-writers-studio":{"element":{"kind":"frame","order":1,"label":"Act I"}}},
			{"id":"link","type":"link","x":240,"y":0,"width":200,"height":120,"url":"https://example.com/research"},
			{"id":"nested","type":"file","x":460,"y":0,"width":200,"height":120,"file":"writers-studio://canvases/other","x-writers-studio":{"resource":{"kind":"canvas","id":"other"}}}
		],
		"edges":[{"id":"shape-link","fromNode":"shape","toNode":"link","toEnd":"arrow"}],
		"x-writers-studio":{"schemaVersion":2}
	}`)
	references, err := Validate(raw)
	if err != nil {
		t.Fatal(err)
	}
	if references.SchemaVersion != 2 || strings.Join(references.CanvasIDs, ",") != "other" {
		t.Fatalf("unexpected v2 references: %#v", references)
	}
}

func TestValidateRejectsInvalidV2ElementsAndLinks(t *testing.T) {
	validText := `{"id":"card","type":"text","x":0,"y":0,"width":100,"height":100,"text":"ok"}`
	validStroke := `{"id":"stroke","type":"group","x":0,"y":0,"width":100,"height":100,"x-writers-studio":{"element":{"kind":"stroke","points":[[1,1],[2,2]],"brush":"pen","color":"1","size":2,"opacity":1}}}`
	tests := map[string]string{
		"invalid document extension": `{"nodes":[],"edges":[],"x-writers-studio":"bad"}`,
		"unsupported schema":         `{"nodes":[],"edges":[],"x-writers-studio":{"schemaVersion":3}}`,
		"missing link url":           `{"nodes":[{"id":"link","type":"link","x":0,"y":0,"width":100,"height":100}],"edges":[]}`,
		"unsafe link protocol":       `{"nodes":[{"id":"link","type":"link","x":0,"y":0,"width":100,"height":100,"url":"file:///secret"}],"edges":[]}`,
		"invalid group extension":    `{"nodes":[{"id":"group","type":"group","x":0,"y":0,"width":100,"height":100,"x-writers-studio":"bad"}],"edges":[]}`,
		"unknown element":            `{"nodes":[{"id":"x","type":"group","x":0,"y":0,"width":100,"height":100,"x-writers-studio":{"element":{"kind":"widget"}}}],"edges":[]}`,
		"short stroke":               `{"nodes":[{"id":"x","type":"group","x":0,"y":0,"width":100,"height":100,"x-writers-studio":{"element":{"kind":"stroke","points":[[1,1]],"brush":"pen","color":"1","size":2,"opacity":1}}}],"edges":[]}`,
		"bad stroke brush":           `{"nodes":[{"id":"x","type":"group","x":0,"y":0,"width":100,"height":100,"x-writers-studio":{"element":{"kind":"stroke","points":[[1,1],[2,2]],"brush":"airbrush","color":"1","size":2,"opacity":1}}}],"edges":[]}`,
		"bad stroke point":           `{"nodes":[{"id":"x","type":"group","x":0,"y":0,"width":100,"height":100,"x-writers-studio":{"element":{"kind":"stroke","points":[[1],[2,2]],"brush":"pen","color":"1","size":2,"opacity":1}}}],"edges":[]}`,
		"bad stroke pressure":        `{"nodes":[{"id":"x","type":"group","x":0,"y":0,"width":100,"height":100,"x-writers-studio":{"element":{"kind":"stroke","points":[[1,1,-1],[2,2]],"brush":"pen","color":"1","size":2,"opacity":1}}}],"edges":[]}`,
		"bad shape":                  `{"nodes":[{"id":"x","type":"group","x":0,"y":0,"width":100,"height":100,"x-writers-studio":{"element":{"kind":"shape","shape":"star","color":"1","fill":"transparent","size":2,"opacity":1,"dash":"solid"}}}],"edges":[]}`,
		"bad shape style":            `{"nodes":[{"id":"x","type":"group","x":0,"y":0,"width":100,"height":100,"x-writers-studio":{"element":{"kind":"shape","shape":"ellipse","color":"bad","fill":"bad","size":0,"opacity":0,"rotation":999,"dash":"waves"}}}],"edges":[]}`,
		"bad frame order":            `{"nodes":[{"id":"x","type":"group","x":0,"y":0,"width":100,"height":100,"x-writers-studio":{"element":{"kind":"frame","order":-1}}}],"edges":[]}`,
		"edge attached to stroke":    `{"nodes":[` + validStroke + `,` + validText + `],"edges":[{"id":"e","fromNode":"stroke","toNode":"card"}]}`,
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Validate(json.RawMessage(raw)); !errors.Is(err, ErrInvalidDocument) {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}

	oversized := append([]byte(`{"padding":"`), bytes.Repeat([]byte("x"), MaxSnapshotBytes)...)
	oversized = append(oversized, []byte(`"}`)...)
	if _, err := Validate(oversized); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("oversized snapshot error = %v", err)
	}
}

func TestValidateRejectsTotalStrokePointLimit(t *testing.T) {
	points := make([][]float64, MaxStrokePoints)
	for index := range points {
		points[index] = []float64{float64(index % 100), float64(index % 100)}
	}
	nodes := make([]node, MaxTotalStrokePoints/MaxStrokePoints+1)
	for index := range nodes {
		extension, err := json.Marshal(resourceExtension{Element: &canvasElement{
			Kind: "stroke", Points: points, Brush: "pen", Color: "1", Size: 2, Opacity: 1,
		}})
		if err != nil {
			t.Fatal(err)
		}
		nodes[index] = node{
			ID: fmt.Sprintf("stroke-%d", index), Type: "group", Width: 100, Height: 100,
			Extension: extension,
		}
	}
	raw, err := json.Marshal(document{Nodes: nodes, Edges: []edge{}, Extension: json.RawMessage(`{"schemaVersion":2}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Validate(raw); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("total stroke point limit error = %v", err)
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

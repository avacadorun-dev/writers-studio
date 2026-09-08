package canvas

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSearchableNodesExtractsStandaloneTypedLinkAndMediaMetadata(t *testing.T) {
	raw := json.RawMessage(`{
		"nodes":[
			{"id":"note","type":"text","x":0,"y":0,"width":100,"height":100,"text":"# Тайный маяк\nСигнал бедствия","x-writers-studio":{"card":{"typeId":"quest","fields":{"reward":"Карта"}}}},
			{"id":"link","type":"link","x":0,"y":0,"width":100,"height":100,"url":"https://example.com/research","x-writers-studio":{"preview":{"title":"Арктическая станция","description":"Исследование"}}},
			{"id":"pdf","type":"file","x":0,"y":0,"width":100,"height":100,"file":"writers-studio://media/media-id","x-writers-studio":{"resource":{"kind":"media","id":"media-id"}}}
		],"edges":[]
	}`)
	result, err := SearchableNodes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 3 || result[0].Title != "Тайный маяк" || !strings.Contains(result[0].Text, "reward") {
		t.Fatalf("result = %#v", result)
	}
	if result[1].Title != "Арктическая станция" || !strings.Contains(result[1].Text, "Исследование") {
		t.Fatalf("link = %#v", result[1])
	}
	if result[2].Resource == nil || result[2].Resource.Kind != "media" {
		t.Fatalf("media = %#v", result[2])
	}
}

func TestSearchableNodesHandlesLabelsAndRejectsMalformedContent(t *testing.T) {
	result, err := SearchableNodes(json.RawMessage(`{"nodes":[{"id":"g","type":"group","x":0,"y":0,"width":100,"height":100,"label":"  Группа "}],"edges":[]}`))
	if err != nil || result[0].Title != "Группа" {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if _, err := SearchableNodes(json.RawMessage(`not-json`)); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("malformed error = %v", err)
	}
	if firstSearchLine("###   ", "fallback") != "fallback" {
		t.Fatal("empty Markdown heading did not use fallback")
	}
}

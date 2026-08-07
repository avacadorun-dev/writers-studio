package assistantapp

import (
	"encoding/json"
	"reflect"
	"testing"

	"writers.studio/internal/editor"
)

func TestCanonicalRichBlocksRoundTripWithoutParagraphFlattening(t *testing.T) {
	t.Parallel()
	assetID := "11111111-1111-1111-1111-111111111111"
	input := map[string]any{"blocks": []any{
		map[string]any{"type": "callout", "kind": "info", "text": "Справка\nбез потерь"},
		map[string]any{"type": "callout", "kind": "idea", "text": "Идея"},
		map[string]any{"type": "callout", "kind": "warning", "text": "Осторожно"},
		map[string]any{"type": "callout", "kind": "todo", "text": "Проверить факты"},
		map[string]any{"type": "mermaid", "code": "flowchart LR\n  A --> B", "caption": "Поток"},
		map[string]any{"type": "image", "assetId": assetID, "alt": "Карта", "caption": "Схема мира"},
		map[string]any{"type": "table", "rows": []any{
			map[string]any{"cells": []any{
				map[string]any{"text": "Параметр", "header": true, "colspan": 1, "rowspan": 1},
				map[string]any{"text": "Значение", "header": true, "colspan": 1, "rowspan": 1},
			}},
			map[string]any{"cells": []any{
				map[string]any{"text": "HP", "header": false, "colspan": 1, "rowspan": 1},
				map[string]any{"text": "100", "header": false, "colspan": 1, "rowspan": 1},
			}},
		}},
		map[string]any{"type": "bullet_list", "items": []any{"Первый", "Второй"}},
		map[string]any{"type": "ordered_list", "items": []any{"Шаг 1", "Шаг 2"}},
		map[string]any{"type": "scene_break"},
		map[string]any{"type": "horizontal_rule"},
	}}

	content, err := normalizeAssistantContent(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := editor.Validate(content); err != nil {
		t.Fatal(err)
	}
	if version, err := editor.SchemaVersion(content); err != nil || version != 2 {
		t.Fatalf("rich schema version = %d, %v", version, err)
	}
	var document editor.Node
	if err := json.Unmarshal(content, &document); err != nil {
		t.Fatal(err)
	}
	if got := []string{
		document.Content[0].Type, document.Content[4].Type,
		document.Content[5].Type, document.Content[6].Type, document.Content[7].Type,
		document.Content[8].Type, document.Content[9].Type, document.Content[10].Type,
	}; !reflect.DeepEqual(got, []string{
		"callout", "mermaidBlock", "imageBlock", "table", "bulletList",
		"orderedList", "sceneBreak", "horizontalRule",
	}) {
		t.Fatalf("canonical rich nodes were flattened: %v", got)
	}
	if document.Content[5].Attrs["src"] != "/api/media/"+assetID+"/content" {
		t.Fatalf("image source is not canonical: %#v", document.Content[5].Attrs)
	}

	blocks, err := assistantBlocksFromDocument(content)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(blocks)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`"type":"callout","text":"Справка\nбез потерь","kind":"info"`,
		`"type":"mermaid","code":"flowchart LR\n  A --\u003e B","caption":"Поток"`,
		`"type":"image","caption":"Схема мира","assetId":"` + assetID + `","alt":"Карта"`,
		`"type":"table","rows":[{"cells":[{"text":"Параметр","header":true,"colspan":1,"rowspan":1}`,
		`{"text":"HP","header":false,"colspan":1,"rowspan":1}`,
		`"type":"bullet_list","items":["Первый","Второй"]`,
		`"type":"ordered_list","items":["Шаг 1","Шаг 2"]`,
		`"type":"scene_break"`,
		`"type":"horizontal_rule"`,
	} {
		if !containsJSONFragment(string(encoded), expected) {
			t.Fatalf("canonical block is missing %s from %s", expected, encoded)
		}
	}
}

func TestCanonicalRichBlocksRejectMalformedPayloads(t *testing.T) {
	t.Parallel()
	cases := []map[string]any{
		{"blocks": []any{map[string]any{"type": "callout", "kind": "danger", "text": "x"}}},
		{"blocks": []any{map[string]any{"type": "mermaid", "code": "", "caption": "x"}}},
		{"blocks": []any{map[string]any{"type": "image", "assetId": "bad", "alt": "x", "caption": ""}}},
		{"blocks": []any{
			map[string]any{"type": "table", "rows": []any{
				map[string]any{"cells": []any{
					map[string]any{"text": "x", "header": false, "colspan": 0, "rowspan": 1},
				}},
			}},
		}},
		{"blocks": []any{map[string]any{"type": "unknown", "text": "x"}}},
	}
	for _, value := range cases {
		if _, err := normalizeAssistantContent(value); err == nil {
			t.Fatalf("malformed rich block was accepted: %#v", value)
		}
	}
}

func containsJSONFragment(document, fragment string) bool {
	return len(fragment) <= len(document) && indexJSONFragment(document, fragment) >= 0
}

func indexJSONFragment(document, fragment string) int {
	for index := 0; index+len(fragment) <= len(document); index++ {
		if document[index:index+len(fragment)] == fragment {
			return index
		}
	}
	return -1
}

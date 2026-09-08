package editor

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"writers.studio/internal/domain"
)

func TestValidateRejectsUnknownNodesAndUnsafeLinks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "unknown node",
			content: `{"type":"doc","content":[{"type":"script","text":"bad"}]}`,
		},
		{
			name: "unsafe link",
			content: `{"type":"doc","content":[{"type":"paragraph","content":[` +
				`{"type":"text","text":"bad","marks":[{"type":"link","attrs":{"href":"javascript:alert(1)"}}]}` +
				`}]}`,
		},
		{
			name:    "unknown schema version",
			content: `{"type":"doc","attrs":{"schemaVersion":5},"content":[{"type":"paragraph"}]}`,
		},
		{
			name:    "unsafe image",
			content: `{"type":"doc","attrs":{"schemaVersion":2},"content":[{"type":"imageBlock","attrs":{"src":"javascript:alert(1)"}}]}`,
		},
		{
			name:    "rich block in legacy schema",
			content: `{"type":"doc","attrs":{"schemaVersion":1},"content":[{"type":"codeBlock","content":[{"type":"text","text":"x"}]}]}`,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := Validate(json.RawMessage(test.content))
			if !errors.Is(err, ErrInvalidDocument) {
				t.Fatalf("expected ErrInvalidDocument, got %v", err)
			}
		})
	}
}

func TestValidateAcceptsCanonicalInternalDocumentLink(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"next","marks":[{"type":"link","attrs":{"href":"/projects/11111111-1111-1111-1111-111111111111/studio/22222222-2222-2222-2222-222222222222#heading"}}]}]}]}`)
	if err := Validate(raw); err != nil {
		t.Fatalf("canonical internal link rejected: %v", err)
	}
}

func TestRichDocumentValidationAndPortableRendering(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{
		"type":"doc","attrs":{"schemaVersion":2},"content":[
			{"type":"paragraph","attrs":{"blockId":"intro"},"content":[
				{"type":"text","text":"HP","marks":[{"type":"code"}]},
				{"type":"text","text":" и баланс"}
			]},
			{"type":"codeBlock","attrs":{"blockId":"code","language":"lua"},"content":[{"type":"text","text":"return hp > 0"}]},
			{"type":"callout","attrs":{"blockId":"idea","kind":"idea"},"content":[
				{"type":"paragraph","attrs":{"blockId":"idea-text"},"content":[{"type":"text","text":"Добавить уклонение"}]}
			]},
			{"type":"table","attrs":{"blockId":"stats"},"content":[
				{"type":"tableRow","content":[
					{"type":"tableHeader","attrs":{"colspan":1,"rowspan":1},"content":[{"type":"paragraph","content":[{"type":"text","text":"Параметр"}]}]},
					{"type":"tableHeader","attrs":{"colspan":1,"rowspan":1},"content":[{"type":"paragraph","content":[{"type":"text","text":"Значение"}]}]}
				]},
				{"type":"tableRow","content":[
					{"type":"tableCell","attrs":{"colspan":1,"rowspan":1},"content":[{"type":"paragraph","content":[{"type":"text","text":"HP"}]}]},
					{"type":"tableCell","attrs":{"colspan":1,"rowspan":1},"content":[{"type":"paragraph","content":[{"type":"text","text":"100"}]}]}
				]}
			]},
			{"type":"imageBlock","attrs":{"blockId":"image","src":"/api/media/11111111-1111-1111-1111-111111111111/content","assetId":"11111111-1111-1111-1111-111111111111","alt":"Карта","caption":"Карта уровня","width":1280,"height":720}},
			{"type":"mermaidBlock","attrs":{"blockId":"flow","code":"flowchart LR\\n  A --> B","caption":"Основной цикл"}}
		]
	}`)
	if err := Validate(raw); err != nil {
		t.Fatalf("rich document rejected: %v", err)
	}
	version, err := SchemaVersion(raw)
	if err != nil || version != 2 {
		t.Fatalf("SchemaVersion() = %d, %v", version, err)
	}
	plain, err := PlainText(raw)
	if err != nil || !strings.Contains(plain, "Карта уровня") ||
		!strings.Contains(plain, "flowchart LR") || !strings.Contains(plain, "100") {
		t.Fatalf("rich plain text = %q, %v", plain, err)
	}
	markdown, err := Markdown(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"`HP`", "```lua", "> [!TIP]", "| Параметр | Значение |",
		"![Карта](/api/media/", "```mermaid", "Основной цикл",
	} {
		if !strings.Contains(markdown, expected) {
			t.Fatalf("Markdown() missing %q in %q", expected, markdown)
		}
	}
}

func TestFromTranscriptJoinsStableSegmentsAndPreservesSourceReferences(t *testing.T) {
	t.Parallel()

	raw, err := FromTranscript([]domain.TranscriptSegment{
		{ID: "segment-a", Text: "Первый абзац."},
		{ID: "segment-b", Text: "Второй абзац."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(raw); err != nil {
		t.Fatalf("generated document is invalid: %v", err)
	}

	var document Node
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Content) != 1 {
		t.Fatalf("provider finals became separate paragraphs: %#v", document.Content)
	}
	sourceRefs := document.Content[0].Attrs["sourceRefs"].([]any)
	if len(sourceRefs) != 2 || sourceRefs[0] != "segment-a" || sourceRefs[1] != "segment-b" {
		t.Fatalf("source refs = %#v", sourceRefs)
	}
	if got := document.Content[0].Content; len(got) != 3 ||
		got[0].Text != "Первый абзац." ||
		got[1].Text != " " ||
		got[2].Text != "Второй абзац." {
		t.Fatalf("joined paragraph = %#v", got)
	}
}

func TestFromTranscriptStartsNewParagraphOnlyAfterCommand(t *testing.T) {
	t.Parallel()

	raw, err := FromTranscript([]domain.TranscriptSegment{
		{ID: "one", Text: "Первое предложение."},
		{ID: "two", Text: "Второе предложение."},
		{
			ID:   "break",
			Text: "Команда, новый абзац",
			Command: &domain.CommandEvent{
				Kind:     "new_paragraph",
				RawText:  "Команда, новый абзац",
				Executed: true,
			},
		},
		{ID: "three", Text: "Настоящий новый абзац."},
	})
	if err != nil {
		t.Fatal(err)
	}
	var document Node
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Content) != 2 {
		t.Fatalf("expected exactly two paragraphs: %#v", document.Content)
	}
	if got := document.Content[1].Content[0].Text; got != "Настоящий новый абзац." {
		t.Fatalf("second paragraph = %q", got)
	}
}

func TestFromTranscriptAppliesOnlyConfirmedStructuralCommands(t *testing.T) {
	t.Parallel()

	raw, err := FromTranscript([]domain.TranscriptSegment{
		{ID: "before", Text: "До команды."},
		{
			ID:   "scene",
			Text: "Команда, разделитель сцены",
			Command: &domain.CommandEvent{
				Kind:     "scene_break",
				RawText:  "Команда, разделитель сцены",
				Executed: true,
			},
		},
		{
			ID:   "unconfirmed",
			Text: "Команда, новый абзац",
			Command: &domain.CommandEvent{
				Kind:     "new_paragraph",
				RawText:  "Команда, новый абзац",
				Executed: false,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var document Node
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Content) != 3 || document.Content[1].Type != "sceneBreak" {
		t.Fatalf("confirmed scene break was not applied: %#v", document.Content)
	}
	if got := document.Content[2].Content[0].Text; got != "Команда, новый абзац" {
		t.Fatalf("unconfirmed command must remain ordinary text, got %q", got)
	}
}

func TestFromTranscriptFormatsRussianDialogueAndDropsTheCommand(t *testing.T) {
	t.Parallel()

	raw, err := FromTranscript([]domain.TranscriptSegment{
		{ID: "before", Text: "Авторская ремарка."},
		{
			ID:   "dialogue",
			Text: "Команда, начало диалога",
			Command: &domain.CommandEvent{
				Kind:     "dialogue_start",
				RawText:  "Команда, начало диалога",
				Executed: true,
			},
		},
		{ID: "line", Text: "Дорогой мистер Беннет, сказала она."},
	})
	if err != nil {
		t.Fatal(err)
	}
	var document Node
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Content) != 2 {
		t.Fatalf("dialogue command leaked into the document: %#v", document.Content)
	}
	if got := document.Content[1].Content[0].Text; got != "— Дорогой мистер Беннет, сказала она." {
		t.Fatalf("dialogue line = %q", got)
	}
}

func TestPlainTextAndMarkdownRenderSemanticContent(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{
		"type":"doc",
		"content":[
			{"type":"heading","attrs":{"level":2,"blockId":"h"},"content":[{"type":"text","text":"Глава"}]},
			{"type":"paragraph","attrs":{"blockId":"p"},"content":[
				{"type":"text","text":"Очень","marks":[{"type":"bold"}]},
				{"type":"text","text":" тихо"}
			]},
			{"type":"sceneBreak"}
		]
	}`)
	plain, err := PlainText(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plain, "Глава") || !strings.Contains(plain, "* * *") {
		t.Fatalf("plain text lost semantic content: %q", plain)
	}
	markdown, err := Markdown(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(markdown, "## Глава") || !strings.Contains(markdown, "**Очень** тихо") {
		t.Fatalf("unexpected markdown: %q", markdown)
	}
}

func TestAnnotationAnchorUsesUnicodeCodePointsAndExactText(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{
		"type":"doc","content":[{
			"type":"paragraph","attrs":{"blockId":"forest"},
			"content":[{"type":"text","text":"Лиса 🦊 вошла"},{"type":"hardBreak"},{"type":"text","text":"домой"}]
		}]
	}`)
	text, found, err := TopLevelBlockText(raw, "forest")
	if err != nil || !found || text != "Лиса 🦊 вошла\nдомой" {
		t.Fatalf("TopLevelBlockText() = %q, %v, %v", text, found, err)
	}
	valid := domain.AnnotationAnchor{
		BlockID: "forest", Start: 7, End: 12, ExactText: "вошла",
	}
	if err := ValidateAnchor(raw, valid); err != nil {
		t.Fatalf("valid Unicode anchor rejected: %v", err)
	}
	for name, anchor := range map[string]domain.AnnotationAnchor{
		"missing block": {BlockID: "missing", Start: 0, End: 1, ExactText: "Л"},
		"outside block": {BlockID: "forest", Start: 7, End: 99, ExactText: "вошла"},
		"text mismatch": {BlockID: "forest", Start: 7, End: 12, ExactText: "ушла"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateAnchor(raw, anchor); !errors.Is(err, ErrInvalidDocument) {
				t.Fatalf("ValidateAnchor() error = %v", err)
			}
		})
	}
	if _, _, err := TopLevelBlockText(json.RawMessage(`{`), "forest"); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("malformed document error = %v", err)
	}
}

func TestReplaceTextPreservesStructureMarksAndUsesLiteralReplacement(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{
		"type":"doc",
		"content":[{
			"type":"paragraph",
			"attrs":{"blockId":"block-1","sourceRefs":["segment-1"]},
			"content":[
				{"type":"text","text":"Анна и АННА","marks":[{"type":"bold"}]},
				{"type":"text","text":" заплатили $5."}
			]
		}]
	}`)
	replaced, count, err := ReplaceText(raw, "анна", "$героиня")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("replacement count = %d, want 2", count)
	}
	var document Node
	if err := json.Unmarshal(replaced, &document); err != nil {
		t.Fatal(err)
	}
	paragraph := document.Content[0]
	if paragraph.Attrs["blockId"] != "block-1" {
		t.Fatalf("block ID changed: %#v", paragraph.Attrs)
	}
	if got := paragraph.Content[0].Text; got != "$героиня и $героиня" {
		t.Fatalf("replacement treated as regexp expansion: %q", got)
	}
	if len(paragraph.Content[0].Marks) != 1 || paragraph.Content[0].Marks[0].Type != "bold" {
		t.Fatalf("marks were lost: %#v", paragraph.Content[0].Marks)
	}
}

func TestReplaceTextCanDeleteTheOnlyTextNode(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(
		`{"type":"doc","content":[{"type":"paragraph","attrs":{"blockId":"p"},"content":[{"type":"text","text":"Удалить"}]}]}`,
	)
	replaced, count, err := ReplaceText(raw, "Удалить", "")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("replacement count = %d, want 1", count)
	}
	if err := Validate(replaced); err != nil {
		t.Fatalf("replacement produced an invalid document: %v", err)
	}
}

func TestNormalizeSchemaVersionUpgradesOnlyRichDocuments(t *testing.T) {
	t.Parallel()

	legacy := json.RawMessage(
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Старый текст"}]}]}`,
	)
	normalized, err := NormalizeSchemaVersion(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if string(normalized) != string(legacy) {
		t.Fatalf("plain legacy document was rewritten: %s", normalized)
	}

	rich := json.RawMessage(
		`{"type":"doc","content":[{"type":"codeBlock","content":[{"type":"text","text":"return true"}]}]}`,
	)
	normalized, err = NormalizeSchemaVersion(rich)
	if err != nil {
		t.Fatal(err)
	}
	version, err := SchemaVersion(normalized)
	if err != nil || version != richSchemaVersion {
		t.Fatalf("normalized schema version = %d, %v", version, err)
	}

	deepHeading := json.RawMessage(
		`{"type":"doc","content":[{"type":"heading","attrs":{"level":6},"content":[{"type":"text","text":"Deep"}]}]}`,
	)
	normalized, err = NormalizeSchemaVersion(deepHeading)
	if err != nil {
		t.Fatal(err)
	}
	version, err = SchemaVersion(normalized)
	if err != nil || version != deepHeadingSchemaVersion {
		t.Fatalf("deep heading schema version = %d, %v", version, err)
	}

	formula := json.RawMessage(
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"inlineMath","attrs":{"latex":"E=mc^2","description":"Энергия"}}]},{"type":"mathBlock","attrs":{"latex":"x^2+y^2","description":""}}]}`,
	)
	normalized, err = NormalizeSchemaVersion(formula)
	if err != nil {
		t.Fatal(err)
	}
	version, err = SchemaVersion(normalized)
	if err != nil || version != mathSchemaVersion {
		t.Fatalf("formula schema version = %d, %v", version, err)
	}
}

func TestFormulaValidationAndPortableRendering(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"type":"doc","attrs":{"schemaVersion":4},"content":[{"type":"paragraph","attrs":{"blockId":"intro"},"content":[{"type":"text","text":"Энергия "},{"type":"inlineMath","attrs":{"latex":"E = mc^2","description":"Энергия равна массе на квадрат скорости света"}}]},{"type":"mathBlock","attrs":{"blockId":"equation","latex":"\\frac{-b \\pm \\sqrt{D}}{2a}","description":"Корни квадратного уравнения"}}]}`)
	if err := Validate(raw); err != nil {
		t.Fatalf("formula document rejected: %v", err)
	}
	plain, err := PlainText(raw)
	if err != nil || !strings.Contains(plain, "[Формула: E = mc^2]") || !strings.Contains(plain, "Формула: \\frac") {
		t.Fatalf("formula plain text = %q, %v", plain, err)
	}
	markdown, err := Markdown(raw)
	if err != nil || !strings.Contains(markdown, "$E = mc^2$") || !strings.Contains(markdown, "$$\n\\frac") {
		t.Fatalf("formula Markdown = %q, %v", markdown, err)
	}

	invalid := []json.RawMessage{
		json.RawMessage(`{"type":"doc","attrs":{"schemaVersion":3},"content":[{"type":"mathBlock","attrs":{"latex":"x"}}]}`),
		json.RawMessage(`{"type":"doc","attrs":{"schemaVersion":4},"content":[{"type":"mathBlock","attrs":{"latex":7}}]}`),
		json.RawMessage(`{"type":"doc","attrs":{"schemaVersion":4},"content":[{"type":"inlineMath","attrs":{"latex":"x"},"content":[{"type":"text","text":"bad"}]}]}`),
	}
	for _, document := range invalid {
		if !errors.Is(Validate(document), ErrInvalidDocument) {
			t.Fatalf("invalid formula accepted: %s", document)
		}
	}
}

func TestFencedCodeMigrationPreservesCodeBytesAndLanguage(t *testing.T) {
	t.Parallel()
	code := "package main  \r\n\r\nfunc main() {\r\n\tprintln(\"hello\")\r\n}\r\n"
	legacyText := "````go\r\n" + code + "\r\n````\r\n"
	parsed, ok := ParseFencedCodeBlock(legacyText)
	if !ok || parsed.Language != "go" || parsed.Text != code {
		t.Fatalf("ParseFencedCodeBlock() = %#v, %v", parsed, ok)
	}

	encodedText, err := json.Marshal(legacyText)
	if err != nil {
		t.Fatal(err)
	}
	legacy := json.RawMessage(`{"type":"doc","attrs":{"schemaVersion":1},"content":[` +
		`{"type":"paragraph","attrs":{"blockId":"legacy","sourceRefs":["mcp"]},` +
		`"content":[{"type":"text","text":` + string(encodedText) + `}]}]}`)
	migrated, err := NormalizeFencedCodeBlocks(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var document Node
	if err := json.Unmarshal(migrated, &document); err != nil {
		t.Fatal(err)
	}
	block := document.Content[0]
	if block.Type != "codeBlock" || block.Attrs["language"] != "go" ||
		block.Attrs["blockId"] != "legacy" || block.Content[0].Text != code {
		t.Fatalf("migrated block changed: %#v", block)
	}
	if version, err := SchemaVersion(migrated); err != nil || version != richSchemaVersion {
		t.Fatalf("migrated schema version = %d, %v", version, err)
	}
}

func TestFencedCodeParserRejectsPartialOrMalformedFences(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"```go\nmissing closing fence",
		"``go\ncode\n``",
		"```bad/language\ncode\n```",
		"```go\ncode\n~~~",
		"text before\n```go\ncode\n```",
	} {
		if parsed, ok := ParseFencedCodeBlock(value); ok {
			t.Fatalf("malformed fence %q parsed as %#v", value, parsed)
		}
	}
	plain := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"not code"}]}]}`)
	unchanged, err := NormalizeFencedCodeBlocks(plain)
	if err != nil || string(unchanged) != string(plain) {
		t.Fatalf("plain document changed: %s, %v", unchanged, err)
	}
}

func TestFencedCodeMigrationHandlesEmptyAndAlreadyRichDocuments(t *testing.T) {
	t.Parallel()
	if _, err := NormalizeFencedCodeBlocks(json.RawMessage(`{`)); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("malformed JSON error = %v", err)
	}
	if _, err := NormalizeFencedCodeBlocks(json.RawMessage(`{"type":"paragraph"}`)); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("non-document root error = %v", err)
	}

	emptyFence := json.RawMessage(`{
		"type":"doc","content":[
			{"type":"heading","attrs":{"level":2}},
			{"type":"paragraph","content":[{"type":"text","text":"` + "```\\n```" + `"}]}
		]
	}`)
	migrated, err := NormalizeFencedCodeBlocks(emptyFence)
	if err != nil {
		t.Fatal(err)
	}
	var document Node
	if err := json.Unmarshal(migrated, &document); err != nil {
		t.Fatal(err)
	}
	if document.Content[1].Type != "codeBlock" || len(document.Content[1].Content) != 0 ||
		document.Attrs["schemaVersion"] != float64(2) {
		t.Fatalf("empty fence migration = %#v", document)
	}

	rich := json.RawMessage(`{
		"type":"doc","attrs":{"schemaVersion":2},"content":[
			{"type":"paragraph","content":[{"type":"text","text":"` + "```go\\ncode\\n```" + `"}]}
		]
	}`)
	migrated, err = NormalizeFencedCodeBlocks(rich)
	if err != nil {
		t.Fatal(err)
	}
	if version, err := SchemaVersion(migrated); err != nil || version != 2 {
		t.Fatalf("rich migration version = %d, %v", version, err)
	}
}

func TestMarkdownUsesSafeFenceWithoutChangingCodeWhitespace(t *testing.T) {
	t.Parallel()
	code := "before ``` middle ```` after\n\tindented  \n"
	raw, err := json.Marshal(Node{
		Type: "doc", Attrs: map[string]any{"schemaVersion": 2},
		Content: []Node{{
			Type: "codeBlock", Attrs: map[string]any{"language": "go"},
			Content: []Node{{Type: "text", Text: code}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	markdown, err := Markdown(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(markdown, "`````go\n") ||
		!strings.Contains(markdown, code+"\n`````") {
		t.Fatalf("unsafe or lossy fenced Markdown: %q", markdown)
	}
}

func TestInlineCodeMigrationPreservesMarksAndLiteralWhitespace(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{
		"type":"doc","attrs":{"schemaVersion":1},"content":[
			{"type":"paragraph","content":[
				{"type":"text","text":"Команды: ` + "`go run`" + `, ` + "``fmt `raw`  ``" + ` и ` + "`go test`" + `.","marks":[{"type":"bold"}]}
			]},
			{"type":"callout","attrs":{"kind":"info"},"content":[
				{"type":"paragraph","content":[{"type":"text","text":"Путь ` + "`a  b\\t c`" + `"}]}
			]},
			{"type":"codeBlock","content":[{"type":"text","text":"` + "`println`" + `"}]},
			{"type":"paragraph","content":[
				{"type":"text","text":"` + "`already marked`" + `","marks":[{"type":"code"}]},
				{"type":"text","text":" and ` + "`unclosed" + `"}
			]}
		]
	}`)
	normalized, err := NormalizeInlineCodeMarks(raw)
	if err != nil {
		t.Fatal(err)
	}
	var document Node
	if err := json.Unmarshal(normalized, &document); err != nil {
		t.Fatal(err)
	}
	if version, err := SchemaVersion(normalized); err != nil || version != richSchemaVersion {
		t.Fatalf("inline code schema version = %d, %v", version, err)
	}
	first := document.Content[0].Content
	if len(first) != 7 || first[1].Text != "go run" || first[3].Text != "fmt `raw`  " ||
		first[5].Text != "go test" {
		t.Fatalf("inline code split = %#v", first)
	}
	for _, index := range []int{1, 3, 5} {
		if !hasMark(first[index].Marks, "bold") || !hasMark(first[index].Marks, "code") {
			t.Fatalf("inline code marks at %d = %#v", index, first[index].Marks)
		}
	}
	calloutText := document.Content[1].Content[0].Content
	if len(calloutText) != 2 || calloutText[1].Text != "a  b\t c" ||
		!hasMark(calloutText[1].Marks, "code") {
		t.Fatalf("nested inline code = %#v", calloutText)
	}
	if got := document.Content[2].Content[0].Text; got != "`println`" {
		t.Fatalf("native code block was changed: %q", got)
	}
	if got := document.Content[3].Content; len(got) != 2 || got[0].Text != "`already marked`" ||
		got[1].Text != " and `unclosed" {
		t.Fatalf("marked or unmatched text changed: %#v", got)
	}
}

func TestInlineCodeParserAndMarkdownUseSafeMatchingDelimiters(t *testing.T) {
	t.Parallel()
	nodes, changed := ParseInlineCodeNodes("before ``a ` b`` after", nil)
	if !changed || len(nodes) != 3 || nodes[1].Text != "a ` b" || !hasMark(nodes[1].Marks, "code") {
		t.Fatalf("ParseInlineCodeNodes() = %#v, %v", nodes, changed)
	}
	unmatched, changed := ParseInlineCodeNodes("before `missing", nil)
	if changed || len(unmatched) != 1 || unmatched[0].Text != "before `missing" {
		t.Fatalf("unmatched inline code changed: %#v, %v", unmatched, changed)
	}
	raw, err := json.Marshal(Node{
		Type: "doc", Attrs: map[string]any{"schemaVersion": 2},
		Content: []Node{{Type: "paragraph", Content: []Node{{
			Type: "text", Text: "a ` b", Marks: []Mark{{Type: "code"}},
		}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	markdown, err := Markdown(raw)
	if err != nil {
		t.Fatal(err)
	}
	if markdown != "``a ` b``" {
		t.Fatalf("inline Markdown delimiter = %q", markdown)
	}
}

func TestInlineCodeMigrationRejectsInvalidDocumentsAndKeepsPlainContentStable(t *testing.T) {
	t.Parallel()
	if _, err := NormalizeInlineCodeMarks(json.RawMessage(`{`)); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("malformed JSON error = %v", err)
	}
	if _, err := NormalizeInlineCodeMarks(json.RawMessage(`{"type":"paragraph"}`)); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("non-document root error = %v", err)
	}
	plain := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"обычный текст"}]}]}`)
	unchanged, err := NormalizeInlineCodeMarks(plain)
	if err != nil || string(unchanged) != string(plain) {
		t.Fatalf("plain content changed: %s, %v", unchanged, err)
	}
}

func TestValidateRequiresMatchingProjectImageAsset(t *testing.T) {
	t.Parallel()

	assetID := "45b17482-b4a0-4624-a68c-d2f218eb4e20"
	valid := json.RawMessage(fmt.Sprintf(
		`{"type":"doc","attrs":{"schemaVersion":2},"content":[{"type":"imageBlock","attrs":{"src":"/api/media/%s/content","assetId":"%s"}}]}`,
		assetID,
		assetID,
	))
	if err := Validate(valid); err != nil {
		t.Fatalf("valid project image rejected: %v", err)
	}
	for name, raw := range map[string]json.RawMessage{
		"missing asset": json.RawMessage(fmt.Sprintf(
			`{"type":"doc","attrs":{"schemaVersion":2},"content":[{"type":"imageBlock","attrs":{"src":"/api/media/%s/content"}}]}`,
			assetID,
		)),
		"mismatched asset": json.RawMessage(fmt.Sprintf(
			`{"type":"doc","attrs":{"schemaVersion":2},"content":[{"type":"imageBlock","attrs":{"src":"/api/media/%s/content","assetId":"850b6344-8a17-4475-aed2-019eb99f96d4"}}]}`,
			assetID,
		)),
		"non-string caption": json.RawMessage(
			`{"type":"doc","attrs":{"schemaVersion":2},"content":[{"type":"imageBlock","attrs":{"src":"https://example.com/image.png","caption":42}}]}`,
		),
	} {
		t.Run(name, func(t *testing.T) {
			if err := Validate(raw); !errors.Is(err, ErrInvalidDocument) {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

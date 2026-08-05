package assistant

import (
	"encoding/json"
	"errors"
	"testing"
)

func testDocument(blocks ...string) json.RawMessage {
	var content []map[string]any
	for index, text := range blocks {
		content = append(content, map[string]any{
			"type": "paragraph",
			"attrs": map[string]any{
				"blockId": "block-" + string(rune('a'+index)), "sourceRefs": []any{},
			},
			"content": []map[string]any{{"type": "text", "text": text}},
		})
	}
	encoded, _ := json.Marshal(map[string]any{"type": "doc", "content": content})
	return encoded
}

func replaceBlockText(document json.RawMessage, blockID string, text string) json.RawMessage {
	var value map[string]any
	_ = json.Unmarshal(document, &value)
	for _, raw := range value["content"].([]any) {
		block := raw.(map[string]any)
		attrs := block["attrs"].(map[string]any)
		if attrs["blockId"] == blockID {
			block["content"] = []any{map[string]any{"type": "text", "text": text}}
		}
	}
	encoded, _ := json.Marshal(value)
	return encoded
}

func TestExplicitTargetKeepsModelSelectedRangesAndProtection(t *testing.T) {
	t.Parallel()
	document := testDocument(
		"Марина вошла.",
		"— Ты пришла.",
		"— Я всегда прихожу.",
	)
	target, err := NewExplicitResolvedTarget(
		"node",
		7,
		document,
		"Сделай диалог менее прямолинейным, но не меняй последнюю реплику",
		[]TextRange{{
			BlockID: "block-b", Start: 0, End: 12, Reason: "selected dialogue line",
		}},
		[]TextRange{{
			BlockID: "block-c", Start: 0, End: 19, Reason: "protected final line",
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(target.Ranges) != 1 || len(target.ProtectedRanges) != 1 ||
		target.ProtectedRanges[0].BlockID != "block-c" {
		t.Fatalf("unexpected target: %+v", target)
	}
	if err := target.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestExplicitTargetDoesNotInterpretNaturalLanguage(t *testing.T) {
	t.Parallel()
	document := testDocument("Первый.", "Второй абзац.", "Третий.")
	selected := []TextRange{{
		BlockID: "block-b", Start: 0, End: 13, Reason: "model-selected range",
	}}
	target, err := NewExplicitResolvedTarget(
		"node",
		3,
		document,
		"Any language, wording or morphology is accepted as provenance only.",
		selected,
		nil,
	)
	if err != nil || len(target.Ranges) != 1 || target.Ranges[0].BlockID != "block-b" {
		t.Fatalf("explicit range validation failed: %+v, %v", target, err)
	}
	target, err = NewExplicitResolvedTarget(
		"node",
		3,
		document,
		"同じ参照先",
		selected,
		nil,
	)
	if err != nil || target.Ranges[0].Start != 0 || target.Ranges[0].End != 13 {
		t.Fatalf("source language affected structural validation: %+v, %v", target, err)
	}
}

func TestExplicitTargetFailureModes(t *testing.T) {
	t.Parallel()
	document := testDocument("Один блок.")
	for _, ranges := range [][]TextRange{
		{{BlockID: "missing", Start: 0, End: 1}},
		{{BlockID: "block-a", Start: 0, End: 99}},
	} {
		if _, err := NewExplicitResolvedTarget(
			"node", 1, document, "provenance", ranges, nil,
		); !errors.Is(err, ErrReferenceNotFound) {
			t.Fatalf("invalid stable range should be rejected: %v", err)
		}
	}
	if _, err := NewExplicitResolvedTarget(
		"node", 1, document, "", []TextRange{{
			BlockID: "block-a", Start: 0, End: 1,
		}}, nil,
	); err == nil {
		t.Fatal("missing source provenance was accepted")
	}
	if _, err := NewExplicitResolvedTarget(
		"node", 1, json.RawMessage(`{"type":"bad"}`), "source",
		[]TextRange{{BlockID: "block-a", Start: 0, End: 1}}, nil,
	); err == nil {
		t.Fatal("invalid document was accepted")
	}
}

func TestValidateProposalScope(t *testing.T) {
	t.Parallel()
	base := testDocument("Вступление.", "— Прямой вопрос.", "— Последний ответ.")
	target, err := NewExplicitResolvedTarget(
		"node",
		1,
		base,
		"Сделай диалог тоньше, но не меняй последнюю реплику",
		[]TextRange{{
			BlockID: "block-b", Start: 0, End: 16, Reason: "selected reply",
		}},
		[]TextRange{{
			BlockID: "block-c", Start: 0, End: 18, Reason: "protected reply",
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	proposed := replaceBlockText(base, "block-b", "— Ты всё-таки здесь?")
	if err := ValidateProposalScope(base, proposed, target); err != nil {
		t.Fatalf("valid scoped proposal rejected: %v", err)
	}
	protectedChanged := replaceBlockText(proposed, "block-c", "— Другой ответ.")
	if err := ValidateProposalScope(
		base, protectedChanged, target,
	); !errors.Is(err, ErrProtectedRangeChanged) {
		t.Fatalf("protected mutation was not rejected: %v", err)
	}
	outsideChanged := replaceBlockText(proposed, "block-a", "Другое вступление.")
	if err := ValidateProposalScope(
		base, outsideChanged, target,
	); !errors.Is(err, ErrMutationOutsideScope) {
		t.Fatalf("out-of-scope mutation was not rejected: %v", err)
	}
}

func TestValidateProposalScopeAllowsProtectedRangeBesideEditInSameBlock(t *testing.T) {
	t.Parallel()
	base := testDocument("Начало OLD середина KEEP конец")
	target, err := NewExplicitResolvedTarget(
		"node",
		1,
		base,
		"Измени только OLD и сохрани KEEP",
		[]TextRange{{BlockID: "block-a", Start: 7, End: 10, Reason: "edit"}},
		[]TextRange{{BlockID: "block-a", Start: 20, End: 24, Reason: "protected"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	proposed := replaceBlockText(base, "block-a", "Начало LONGER середина KEEP конец")
	if err := ValidateProposalScope(base, proposed, target); err != nil {
		t.Fatalf("non-overlapping protected range in edited block was rejected: %v", err)
	}
	overlapping := target
	overlapping.ProtectedRanges = []TextRange{{
		BlockID: "block-a", Start: 8, End: 12, Reason: "overlap",
	}}
	if err := ValidateProposalScope(base, proposed, overlapping); !errors.Is(
		err, ErrProtectedRangeChanged,
	) {
		t.Fatalf("overlapping protected range was not rejected: %v", err)
	}
}

func TestResolvedTargetValidation(t *testing.T) {
	t.Parallel()
	valid := ResolvedTarget{
		NodeID: "node", BaseRevision: 1, SourcePhrase: "Измени",
		Ranges: []TextRange{{BlockID: "block", Start: 0, End: 2}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := []ResolvedTarget{
		{},
		{NodeID: "node", BaseRevision: -1, SourcePhrase: "x", Ranges: valid.Ranges},
		{NodeID: "node", SourcePhrase: "x", Ranges: []TextRange{{Start: 0, End: 1}}},
		{NodeID: "node", SourcePhrase: "x", Ranges: []TextRange{{
			BlockID: "block", Start: 2, End: 1,
		}}},
	}
	for _, target := range invalid {
		if err := target.Validate(); err == nil {
			t.Fatalf("invalid target accepted: %+v", target)
		}
	}
}

func TestResolverHelpers(t *testing.T) {
	t.Parallel()
	if text := inlineText([]inlineNode{{
		Text: "a", Content: []inlineNode{{Text: "b"}},
	}}); text != "ab" {
		t.Fatalf("unexpected inline text: %q", text)
	}
	if !outsideRangesEqual("abcXYZdef", "abcNEWdef", []TextRange{{
		Start: 3, End: 6,
	}}) {
		t.Fatal("valid middle replacement rejected")
	}
	if outsideRangesEqual("abcXYZdef", "badNEWdef", []TextRange{{
		Start: 3, End: 6,
	}}) {
		t.Fatal("changed prefix accepted")
	}
	if !outsideRangesEqual("aaOLD--SECONDzz", "aaLONGER--Xzz", []TextRange{
		{Start: 7, End: 13},
		{Start: 2, End: 5},
	}) {
		t.Fatal("two disjoint replacements with different lengths were rejected")
	}
	if outsideRangesEqual("aaOLD--SECONDzz", "aaLONGER changed Xzz", []TextRange{
		{Start: 2, End: 5},
		{Start: 7, End: 13},
	}) {
		t.Fatal("changed text between two allowed ranges was accepted")
	}
	if !outsideRangesEqual("aXYz", "a12z", []TextRange{
		{Start: 1, End: 2},
		{Start: 2, End: 3},
	}) {
		t.Fatal("adjacent replacements were rejected")
	}
	if outsideRangesEqual("abcdef", "abXYef", []TextRange{
		{Start: 2, End: 4},
		{Start: 3, End: 5},
	}) {
		t.Fatal("overlapping allowed ranges were accepted")
	}
}

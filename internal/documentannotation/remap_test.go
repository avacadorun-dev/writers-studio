package documentannotation

import (
	"testing"

	"writers.studio/internal/domain"
)

func TestRemapAnchor(t *testing.T) {
	anchor := domain.AnnotationAnchor{BlockID: "b", Start: 10, End: 14, ExactText: "text"}
	tests := []struct {
		name     string
		edits    []TextEdit
		start    int
		end      int
		exact    string
		detached bool
	}{
		{name: "unrelated block", edits: []TextEdit{{BlockID: "x", Start: 0, End: 1, Expected: "a", Replacement: "long"}}, start: 10, end: 14, exact: "text"},
		{name: "shift before", edits: []TextEdit{{BlockID: "b", Start: 1, End: 3, Expected: "ab", Replacement: "12345"}}, start: 13, end: 17, exact: "text"},
		{name: "edit after", edits: []TextEdit{{BlockID: "b", Start: 20, End: 21, Expected: "x", Replacement: ""}}, start: 10, end: 14, exact: "text"},
		{name: "exact replacement", edits: []TextEdit{{BlockID: "b", Start: 10, End: 14, Expected: "text", Replacement: "новый"}}, start: 10, end: 15, exact: "новый"},
		{name: "shift and replace", edits: []TextEdit{{BlockID: "b", Start: 0, End: 1, Expected: "x", Replacement: "xxx"}, {BlockID: "b", Start: 10, End: 14, Expected: "text", Replacement: "ok"}}, start: 12, end: 14, exact: "ok"},
		{name: "partial overlap", edits: []TextEdit{{BlockID: "b", Start: 8, End: 12, Expected: "xxxx", Replacement: "y"}}, detached: true},
		{name: "contained edit", edits: []TextEdit{{BlockID: "b", Start: 11, End: 12, Expected: "e", Replacement: "E"}}, detached: true},
		{name: "deletion", edits: []TextEdit{{BlockID: "b", Start: 10, End: 14, Expected: "text", Replacement: ""}}, detached: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mapped, detached, err := RemapAnchor(anchor, test.edits)
			if err != nil || detached != test.detached {
				t.Fatalf("detached=%v err=%v", detached, err)
			}
			if detached {
				return
			}
			if mapped.Start != test.start || mapped.End != test.end || mapped.ExactText != test.exact {
				t.Fatalf("mapped=%+v", mapped)
			}
		})
	}
}

func TestRemapAnchorRejectsInvalidEdits(t *testing.T) {
	anchor := domain.AnnotationAnchor{BlockID: "b", Start: 2, End: 3, ExactText: "x"}
	for _, edits := range [][]TextEdit{
		{{BlockID: "", Start: 0, End: 1, Expected: "x"}},
		{{BlockID: "b", Start: 1, End: 0}},
		{{BlockID: "b", Start: 0, End: 2, Expected: "x"}},
		{{BlockID: "b", Start: 0, End: 2, Expected: "xx"}, {BlockID: "b", Start: 1, End: 3, Expected: "xx"}},
	} {
		if _, _, err := RemapAnchor(anchor, edits); err == nil {
			t.Fatal("expected invalid edit error")
		}
	}
}

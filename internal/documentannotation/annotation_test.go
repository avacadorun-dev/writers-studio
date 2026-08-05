package documentannotation

import (
	"errors"
	"strings"
	"testing"

	"writers.studio/internal/domain"
)

func TestValidateCreateCoversKindsAndAnchorContract(t *testing.T) {
	t.Parallel()
	validAnchor := domain.AnnotationAnchor{BlockID: "block", Start: 1, End: 3, ExactText: "да"}
	valid := []struct {
		kind, note string
		hasTerm    bool
	}{
		{kind: "note", note: "  Проверить  "},
		{kind: "dictionary_link", hasTerm: true},
	}
	for _, input := range valid {
		if err := ValidateCreate(input.kind, input.hasTerm, input.note, 1, validAnchor); err != nil {
			t.Fatalf("valid %s rejected: %v", input.kind, err)
		}
	}

	invalid := []struct {
		name, kind, note string
		hasTerm          bool
		revision         int
		anchor           domain.AnnotationAnchor
	}{
		{name: "kind", kind: "other", note: "x", revision: 1, anchor: validAnchor},
		{name: "revision", kind: "note", note: "x", revision: 0, anchor: validAnchor},
		{name: "block", kind: "note", note: "x", revision: 1, anchor: domain.AnnotationAnchor{Start: 0, End: 1, ExactText: "x"}},
		{name: "text", kind: "note", note: "x", revision: 1, anchor: domain.AnnotationAnchor{BlockID: "block", Start: 0, End: 1}},
		{name: "negative", kind: "note", note: "x", revision: 1, anchor: domain.AnnotationAnchor{BlockID: "block", Start: -1, End: 1, ExactText: "x"}},
		{name: "empty range", kind: "note", note: "x", revision: 1, anchor: domain.AnnotationAnchor{BlockID: "block", Start: 1, End: 1, ExactText: "x"}},
		{name: "note without text", kind: "note", note: "  ", revision: 1, anchor: validAnchor},
		{name: "long note", kind: "note", note: strings.Repeat("я", MaxNoteRunes+1), revision: 1, anchor: validAnchor},
		{name: "note with term", kind: "note", note: "x", hasTerm: true, revision: 1, anchor: validAnchor},
		{name: "link without term", kind: "dictionary_link", revision: 1, anchor: validAnchor},
		{name: "link with note", kind: "dictionary_link", note: "x", hasTerm: true, revision: 1, anchor: validAnchor},
	}
	for _, input := range invalid {
		t.Run(input.name, func(t *testing.T) {
			if err := ValidateCreate(input.kind, input.hasTerm, input.note, input.revision, input.anchor); !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestNormalizeNoteAndStatusValidation(t *testing.T) {
	t.Parallel()
	if got, err := NormalizeNote("  Тише  "); err != nil || got != "Тише" {
		t.Fatalf("NormalizeNote() = %q, %v", got, err)
	}
	for _, note := range []string{"", "  ", strings.Repeat("я", MaxNoteRunes+1)} {
		if _, err := NormalizeNote(note); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid note error = %v", err)
		}
	}
	for _, status := range []string{"open", "resolved", "needs_attention"} {
		if !ValidStatus(status) || ValidateRequestedStatus("note", false, status) != nil {
			t.Fatalf("valid status rejected: %q", status)
		}
	}
	if ValidStatus("closed") || !errors.Is(ValidateRequestedStatus("note", false, "closed"), ErrInvalid) {
		t.Fatal("invalid status accepted")
	}
	if !errors.Is(ValidateRequestedStatus("dictionary_link", false, "open"), ErrInvalid) {
		t.Fatal("deleted dictionary target reopened")
	}
	if err := ValidateRequestedStatus("dictionary_link", false, "resolved"); err != nil {
		t.Fatalf("deleted link cannot be resolved: %v", err)
	}
}

func TestStatusAfterMappingCoversEveryTransition(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, current, kind, want string
		hasTerm, detached, valid  bool
	}{
		{name: "resolved is stable", current: "resolved", kind: "note", detached: true, want: "resolved"},
		{name: "detached", current: "open", kind: "note", detached: true, valid: true, want: "needs_attention"},
		{name: "invalid", current: "open", kind: "note", want: "needs_attention"},
		{name: "reattach note", current: "needs_attention", kind: "note", valid: true, want: "open"},
		{name: "reattach live link", current: "needs_attention", kind: "dictionary_link", hasTerm: true, valid: true, want: "open"},
		{name: "deleted link remains", current: "needs_attention", kind: "dictionary_link", valid: true, want: "needs_attention"},
		{name: "open remains", current: "open", kind: "note", valid: true, want: "open"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := StatusAfterMapping(test.current, test.kind, test.hasTerm, test.detached, test.valid); got != test.want {
				t.Fatalf("status = %q, want %q", got, test.want)
			}
		})
	}
}

func TestStatusAfterCopyPreservesOrFlagsSafely(t *testing.T) {
	t.Parallel()
	tests := []struct {
		current          string
		mapped, preserve bool
		want             string
	}{
		{current: "open", mapped: true, want: "open"},
		{current: "open", preserve: true, want: "open"},
		{current: "open", want: "needs_attention"},
		{current: "resolved", want: "resolved"},
	}
	for _, test := range tests {
		if got := StatusAfterCopy(test.current, test.mapped, test.preserve); got != test.want {
			t.Fatalf("StatusAfterCopy(%q, %v, %v) = %q, want %q", test.current, test.mapped, test.preserve, got, test.want)
		}
	}
}

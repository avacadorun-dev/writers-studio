package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestMoveDocumentNodeToIndexRejectsNegativeIndexBeforeStartingTransaction(t *testing.T) {
	t.Parallel()
	dataStore := &Store{}
	err := dataStore.MoveDocumentNodeToIndex(context.Background(), "owner", "node", nil, -1)
	if !errors.Is(err, ErrInvalidDocumentMove) {
		t.Fatalf("error = %v, want ErrInvalidDocumentMove", err)
	}
}

func TestSearchSnippetPreservesUTF8Boundaries(t *testing.T) {
	t.Parallel()

	text := strings.Repeat("До реки было далеко. ", 10) +
		"Анна остановилась у калитки." +
		strings.Repeat(" Дом молчал.", 20)
	snippet := searchSnippet(text, "анна")
	if !utf8.ValidString(snippet) {
		t.Fatalf("snippet is not valid UTF-8: %q", snippet)
	}
	if !strings.Contains(snippet, "Анна остановилась") {
		t.Fatalf("snippet does not contain the match: %q", snippet)
	}
	if len([]rune(snippet)) >= len([]rune(text)) {
		t.Fatalf("expected a bounded snippet, got the full document")
	}
}

func TestDictationTransitionTableRejectsUnsafeJumps(t *testing.T) {
	t.Parallel()

	allowed := [][2]string{
		{"recording_online", "recording_degraded"},
		{"recording_online", "paused"},
		{"paused", "recording_online"},
		{"recording_degraded", "stopping"},
	}
	for _, transition := range allowed {
		if !validDictationTransition(transition[0], transition[1]) {
			t.Errorf("expected %s -> %s to be allowed", transition[0], transition[1])
		}
	}
	rejected := [][2]string{
		{"review_ready", "recording_online"},
		{"finalizing", "paused"},
		{"recording_online", "review_ready"},
	}
	for _, transition := range rejected {
		if validDictationTransition(transition[0], transition[1]) {
			t.Errorf("expected %s -> %s to be rejected", transition[0], transition[1])
		}
	}
}

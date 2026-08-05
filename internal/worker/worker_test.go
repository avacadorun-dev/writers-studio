package worker

import (
	"testing"

	"writers.studio/internal/domain"
	"writers.studio/internal/store"
	"writers.studio/internal/stt"
)

func TestNormalizeFinalCommandsRemovesSpokenCommandAndFormatsDialogue(t *testing.T) {
	t.Parallel()

	final := stt.FinalResult{
		Text: "Ремарка. Команда «Начало диалога». Дорогой мистер Беннет.",
		Segments: []domain.TranscriptSegment{{
			ID:      "batch",
			Text:    "Ремарка. Команда «Начало диалога». Дорогой мистер Беннет.",
			StartMS: 0,
			EndMS:   5_000,
		}},
	}
	err := normalizeFinalCommands(&final, store.WorkerSessionMeta{
		CommandsEnabled: true,
		CommandPrefix:   "Команда",
	})
	if err != nil {
		t.Fatal(err)
	}
	if final.Text != "Ремарка.\n\n— Дорогой мистер Беннет." {
		t.Fatalf("normalized text = %q", final.Text)
	}
	if len(final.Segments) != 3 ||
		final.Segments[1].Command == nil ||
		final.Segments[1].Command.Kind != "dialogue_start" {
		t.Fatalf("command metadata was not preserved: %#v", final.Segments)
	}
}

func TestNormalizeFinalCommandsLeavesSpeechUntouchedWhenCommandsAreDisabled(t *testing.T) {
	t.Parallel()

	final := stt.FinalResult{
		Text: "Команда новый абзац.",
		Segments: []domain.TranscriptSegment{{
			ID:      "batch",
			Text:    "Команда новый абзац.",
			StartMS: 0,
			EndMS:   1_000,
		}},
	}
	err := normalizeFinalCommands(&final, store.WorkerSessionMeta{
		CommandsEnabled: false,
		CommandPrefix:   "Команда",
	})
	if err != nil {
		t.Fatal(err)
	}
	if final.Text != "Команда новый абзац." || len(final.Segments) != 1 {
		t.Fatalf("disabled commands changed the transcript: %#v", final)
	}
}

func TestNormalizeFinalCommandsJoinsProviderSegmentsWithoutParagraphCommands(t *testing.T) {
	t.Parallel()

	final := stt.FinalResult{
		Text: "Первое предложение.\n\nВторое предложение.",
		Segments: []domain.TranscriptSegment{
			{ID: "one", Text: "Первое предложение.", StartMS: 0, EndMS: 1_000},
			{ID: "two", Text: "Второе предложение.", StartMS: 1_000, EndMS: 2_000},
		},
	}
	err := normalizeFinalCommands(&final, store.WorkerSessionMeta{
		CommandsEnabled: true,
		CommandPrefix:   "Команда",
	})
	if err != nil {
		t.Fatal(err)
	}
	if final.Text != "Первое предложение. Второе предложение." {
		t.Fatalf("provider segment boundaries leaked into plain text: %q", final.Text)
	}
}

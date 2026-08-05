package dictation

import (
	"testing"

	"writers.studio/internal/domain"
)

func TestParseCommandRequiresExactPrefixAndGrammar(t *testing.T) {
	t.Parallel()

	confident := 0.9
	command := ParseCommand("Команда, новый абзац", "Команда", true, &confident)
	if command == nil || command.Kind != "new_paragraph" || !command.Executed {
		t.Fatalf("expected executed paragraph command, got %#v", command)
	}
	if ParseCommand("Новый абзац", "Команда", true, &confident) != nil {
		t.Fatal("phrase without prefix must remain text")
	}
	if ParseCommand("Команда, придумай продолжение", "Команда", true, &confident) != nil {
		t.Fatal("free-form instruction must not execute")
	}
	if ParseCommand("Команда, новый абзац", "Команда", false, &confident) != nil {
		t.Fatal("disabled commands must remain text")
	}
}

func TestParseCommandDoesNotExecuteLowConfidence(t *testing.T) {
	t.Parallel()

	low := 0.4
	command := ParseCommand("Команда, разделитель сцены", "Команда", true, &low)
	if command == nil || command.Kind != "scene_break" || command.Executed {
		t.Fatalf("low-confidence command must be suggested, got %#v", command)
	}
}

func TestParseCommandRecognizesRussianDialogueStart(t *testing.T) {
	t.Parallel()

	confident := 0.96
	for _, phrase := range []string{
		"Команда, начало диалога.",
		"Команда: начало реплики",
		"Команда — новая реплика",
	} {
		command := ParseCommand(phrase, "Команда", true, &confident)
		if command == nil || command.Kind != "dialogue_start" || !command.Executed {
			t.Fatalf("expected dialogue start for %q, got %#v", phrase, command)
		}
	}
}

func TestParseCommandRecognizesNavigationTargets(t *testing.T) {
	t.Parallel()

	confident := 0.96
	cases := []struct {
		phrase  string
		kind    string
		ordinal int
		target  string
	}{
		{"Команда, вернись к первому абзацу", "navigate_paragraph", 1, ""},
		{"Команда, вернись к 2-му абзацу", "navigate_paragraph", 2, ""},
		{"Команда, вернись к третьему предложению", "navigate_sentence", 3, ""},
		{
			"Команда, вернись к предложению зубы у него заболели",
			"navigate_sentence",
			0,
			"зубы у него заболели",
		},
		{
			"Команда, вернись к словам ел много сладкого",
			"navigate_text",
			0,
			"ел много сладкого",
		},
		{"Команда, вернись в конец", "navigate_end", 0, ""},
	}
	for _, testCase := range cases {
		command := ParseCommand(testCase.phrase, "Команда", true, &confident)
		if command == nil ||
			command.Kind != testCase.kind ||
			command.Ordinal != testCase.ordinal ||
			command.Target != testCase.target ||
			!command.Executed {
			t.Fatalf("unexpected navigation command for %q: %#v", testCase.phrase, command)
		}
	}
}

func TestParseNavigationCommandHonorsConfidence(t *testing.T) {
	t.Parallel()

	low := 0.4
	command := ParseCommand(
		"Команда, вернись к первому абзацу",
		"Команда",
		true,
		&low,
	)
	if command == nil || command.Kind != "navigate_paragraph" || command.Executed {
		t.Fatalf("low-confidence navigation must not execute: %#v", command)
	}
}

func TestCommandKeytermsIncludeStructuralPhrases(t *testing.T) {
	t.Parallel()

	terms := CommandKeyterms("Команда", true)
	if len(terms) == 0 || terms[2] != "Команда начало диалога" {
		t.Fatalf("unexpected command keyterms: %#v", terms)
	}
	if CommandKeyterms("Команда", false) != nil {
		t.Fatal("disabled commands must not add keyterms")
	}
}

func TestRecognizeCommandsInBatchTranscriptRemovesInlineCommand(t *testing.T) {
	t.Parallel()

	segments := RecognizeCommandsInTranscript([]domain.TranscriptSegment{{
		ID:      "batch",
		Text:    "Первый абзац. Команда «Новый абзац». Второй абзац.",
		StartMS: 0,
		EndMS:   5_000,
	}}, "Команда", true)
	if len(segments) != 3 {
		t.Fatalf("expected prose, command, prose; got %#v", segments)
	}
	if segments[1].Command == nil ||
		segments[1].Command.Kind != "new_paragraph" ||
		!segments[1].Command.Executed {
		t.Fatalf("inline command was not recognized: %#v", segments[1])
	}
	if segments[0].Text != "Первый абзац." || segments[2].Text != "Второй абзац." {
		t.Fatalf("prose changed while extracting command: %#v", segments)
	}
}

func TestRecognizeCommandsInBatchTranscriptExtractsMarkerText(t *testing.T) {
	t.Parallel()

	segments := RecognizeCommandsInTranscript([]domain.TranscriptSegment{{
		ID:      "batch",
		Text:    "До. Команда, пометка проверить имя героя. После.",
		StartMS: 0,
		EndMS:   4_000,
	}}, "Команда", true)
	if len(segments) != 3 ||
		segments[1].Command == nil ||
		segments[1].Command.Kind != "marker" ||
		!segments[1].Command.Executed {
		t.Fatalf("marker command was not extracted: %#v", segments)
	}
	if segments[0].Text != "До." || segments[2].Text != "После." {
		t.Fatalf("marker extraction changed prose: %#v", segments)
	}
}

func TestRecognizeCommandsInBatchTranscriptExtractsNavigation(t *testing.T) {
	t.Parallel()

	segments := RecognizeCommandsInTranscript([]domain.TranscriptSegment{{
		ID:      "batch",
		Text:    "Дракон спал. Команда, вернись к первому абзацу. Потом проснулся.",
		StartMS: 0,
		EndMS:   5_000,
	}}, "Команда", true)
	if len(segments) != 3 ||
		segments[1].Command == nil ||
		segments[1].Command.Kind != "navigate_paragraph" ||
		segments[1].Command.Ordinal != 1 ||
		!segments[1].Command.Executed {
		t.Fatalf("navigation command was not extracted: %#v", segments)
	}
	if segments[0].Text != "Дракон спал." || segments[2].Text != "Потом проснулся." {
		t.Fatalf("navigation extraction changed prose: %#v", segments)
	}
}

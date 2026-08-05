package dictation

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"writers.studio/internal/domain"
)

func ParseCommand(text, prefix string, enabled bool, confidence *float64) *domain.CommandEvent {
	if !enabled {
		return nil
	}
	normalizedText := normalize(text)
	normalizedPrefix := normalize(prefix)
	if normalizedPrefix == "" || !strings.HasPrefix(normalizedText, normalizedPrefix+" ") {
		return nil
	}

	commandText := strings.TrimSpace(strings.TrimPrefix(normalizedText, normalizedPrefix))
	command := &domain.CommandEvent{RawText: text}
	switch {
	case commandText == "новый абзац" || commandText == "new paragraph":
		command.Kind = "new_paragraph"
	case commandText == "начало диалога" ||
		commandText == "начало реплики" ||
		commandText == "новая реплика" ||
		commandText == "dialogue start" ||
		commandText == "start dialogue":
		command.Kind = "dialogue_start"
	case commandText == "разделитель сцены" || commandText == "scene break":
		command.Kind = "scene_break"
	case commandText == "отменить команду" || commandText == "undo command":
		command.Kind = "undo_command"
	case parseNavigationCommand(commandText, command):
	case commandText == "пометка" ||
		commandText == "note" ||
		strings.HasPrefix(commandText, "пометка ") ||
		strings.HasPrefix(commandText, "note "):
		command.Kind = "marker"
	default:
		return nil
	}
	command.Executed = confidence == nil || *confidence >= 0.75
	return command
}

func CommandKeyterms(prefix string, enabled bool) []string {
	prefix = strings.TrimSpace(prefix)
	if !enabled || prefix == "" {
		return nil
	}
	return []string{
		prefix,
		prefix + " новый абзац",
		prefix + " начало диалога",
		prefix + " начало реплики",
		prefix + " разделитель сцены",
		prefix + " пометка",
		prefix + " отменить команду",
		prefix + " вернись к первому абзацу",
		prefix + " вернись к предложению",
		prefix + " вернись к словам",
		prefix + " вернись в конец",
	}
}

func RecognizeCommandsInTranscript(
	segments []domain.TranscriptSegment,
	prefix string,
	enabled bool,
) []domain.TranscriptSegment {
	prefix = strings.TrimSpace(prefix)
	if !enabled || prefix == "" {
		return segments
	}
	phrases := strings.Join([]string{
		`новый\s+абзац`,
		`начало\s+диалога`,
		`начало\s+реплики`,
		`новая\s+реплика`,
		`разделитель\s+сцены`,
		`пометка(?:\s+[^.!?…\n]+)?`,
		`отменить\s+команду`,
		`(?:вернись|перейди)\s+(?:к|в|на)\s+(?:первому|второму|третьему|четв[её]ртому|пятому|шестому|седьмому|восьмому|девятому|десятому|[1-9]|10)(?:-?(?:й|ый|му))?\s+абзац(?:у|е)?`,
		`(?:вернись|перейди)\s+(?:к|в|на)\s+абзац(?:у|е)?\s+(?:[1-9]|10)`,
		`(?:вернись|перейди)\s+к\s+(?:первому|второму|третьему|четв[её]ртому|пятому|шестому|седьмому|восьмому|девятому|десятому|[1-9]|10)(?:-?(?:й|ый|му))?\s+(?:предложению|фразе)`,
		`(?:вернись|перейди)\s+к\s+(?:предложению|фразе)\s+[^.!?…\n]+`,
		`(?:вернись|перейди)\s+к\s+(?:месту|словам|тексту)\s+[^.!?…\n]+`,
		`(?:вернись|перейди)\s+(?:в\s+конец|к\s+концу)`,
		`new\s+paragraph`,
		`dialogue\s+start`,
		`start\s+dialogue`,
		`scene\s+break`,
		`note(?:\s+[^.!?\n]+)?`,
		`undo\s+command`,
	}, "|")
	expression := regexp.MustCompile(
		`(?i)(^|[\s\p{P}])(` +
			regexp.QuoteMeta(prefix) +
			`[\s\p{P}]*(` +
			phrases +
			`)[\s\p{P}]*)`,
	)

	result := make([]domain.TranscriptSegment, 0, len(segments))
	for _, segment := range segments {
		matches := expression.FindAllStringSubmatchIndex(segment.Text, -1)
		if len(matches) == 0 {
			result = append(result, segment)
			continue
		}
		cursor := 0
		for _, match := range matches {
			commandStart, commandEnd := match[4], match[5]
			phraseStart, phraseEnd := match[6], match[7]
			result = appendTranscriptPiece(result, segment, cursor, commandStart, nil)
			command := ParseCommand(
				prefix+" "+segment.Text[phraseStart:phraseEnd],
				prefix,
				enabled,
				segment.Confidence,
			)
			if command != nil {
				command.RawText = strings.TrimSpace(segment.Text[commandStart:commandEnd])
				result = appendTranscriptPiece(
					result,
					segment,
					commandStart,
					commandEnd,
					command,
				)
			}
			cursor = commandEnd
		}
		result = appendTranscriptPiece(result, segment, cursor, len(segment.Text), nil)
	}
	for index := range result {
		command := result[index].Command
		if command == nil || !command.Executed || command.Kind != "undo_command" {
			continue
		}
		for previous := index - 1; previous >= 0; previous-- {
			candidate := result[previous].Command
			if candidate != nil && candidate.Executed && candidate.Kind != "undo_command" {
				candidate.Executed = false
				break
			}
		}
	}
	return result
}

func appendTranscriptPiece(
	target []domain.TranscriptSegment,
	source domain.TranscriptSegment,
	start, end int,
	command *domain.CommandEvent,
) []domain.TranscriptSegment {
	text := strings.TrimSpace(source.Text[start:end])
	if text == "" {
		return target
	}
	startMS := transcriptOffsetMS(source, start)
	endMS := transcriptOffsetMS(source, end)
	if endMS <= startMS {
		endMS = startMS + 1
	}
	words := make([]domain.TranscriptWord, 0)
	for _, word := range source.Words {
		if word.StartMS >= startMS && word.EndMS <= endMS {
			words = append(words, word)
		}
	}
	return append(target, domain.TranscriptSegment{
		ID:         uuid.NewString(),
		Text:       text,
		StartMS:    startMS,
		EndMS:      endMS,
		Confidence: source.Confidence,
		Words:      words,
		Command:    command,
	})
}

func transcriptOffsetMS(segment domain.TranscriptSegment, byteOffset int) int64 {
	totalRunes := utf8.RuneCountInString(segment.Text)
	if totalRunes == 0 || segment.EndMS <= segment.StartMS {
		return segment.StartMS
	}
	offsetRunes := utf8.RuneCountInString(segment.Text[:byteOffset])
	return segment.StartMS +
		(segment.EndMS-segment.StartMS)*int64(offsetRunes)/int64(totalRunes)
}

func normalize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Map(func(char rune) rune {
		if unicode.IsPunct(char) && char != '-' {
			return ' '
		}
		return char
	}, value)
	return strings.Join(strings.Fields(value), " ")
}

var (
	navigationVerbPattern = `(?:вернись|перейди|вернуться|перейти|return|go)`
	navigateEndExpression = regexp.MustCompile(
		`^` + navigationVerbPattern + `\s+(?:в\s+конец|к\s+концу|to\s+(?:the\s+)?end)$`,
	)
	navigateParagraphExpression = regexp.MustCompile(
		`^` + navigationVerbPattern + `\s+(?:к|в|на)\s+(.+?)\s+абзац(?:у|е)?$`,
	)
	navigateParagraphAfterExpression = regexp.MustCompile(
		`^` + navigationVerbPattern + `\s+(?:к|в|на)\s+абзац(?:у|е)?\s+(.+)$`,
	)
	navigateSentenceExpression = regexp.MustCompile(
		`^` + navigationVerbPattern + `\s+к\s+(.+?)\s+(?:предложению|фразе)$`,
	)
	navigateSentenceTargetExpression = regexp.MustCompile(
		`^` + navigationVerbPattern + `\s+к\s+(?:предложению|фразе)\s+(.+)$`,
	)
	navigateTextExpression = regexp.MustCompile(
		`^` + navigationVerbPattern + `\s+к\s+(?:месту|словам|тексту)\s+(.+)$`,
	)
)

func parseNavigationCommand(commandText string, command *domain.CommandEvent) bool {
	if navigateEndExpression.MatchString(commandText) {
		command.Kind = "navigate_end"
		return true
	}
	for _, expression := range []*regexp.Regexp{
		navigateParagraphExpression,
		navigateParagraphAfterExpression,
	} {
		match := expression.FindStringSubmatch(commandText)
		if len(match) != 2 {
			continue
		}
		ordinal, ok := parseOrdinal(match[1])
		if !ok {
			return false
		}
		command.Kind = "navigate_paragraph"
		command.Ordinal = ordinal
		return true
	}
	if match := navigateSentenceExpression.FindStringSubmatch(commandText); len(match) == 2 {
		ordinal, ok := parseOrdinal(match[1])
		if !ok {
			return false
		}
		command.Kind = "navigate_sentence"
		command.Ordinal = ordinal
		return true
	}
	if match := navigateSentenceTargetExpression.FindStringSubmatch(commandText); len(match) == 2 {
		command.Kind = "navigate_sentence"
		command.Target = strings.TrimSpace(match[1])
		return command.Target != ""
	}
	if match := navigateTextExpression.FindStringSubmatch(commandText); len(match) == 2 {
		command.Kind = "navigate_text"
		command.Target = strings.TrimSpace(match[1])
		return command.Target != ""
	}
	return false
}

func parseOrdinal(value string) (int, bool) {
	value = strings.TrimSpace(value)
	ordinals := map[string]int{
		"1": 1, "1-й": 1, "1-ый": 1, "1-му": 1, "первый": 1, "первому": 1,
		"2": 2, "2-й": 2, "2-ой": 2, "2-му": 2, "второй": 2, "второму": 2,
		"3": 3, "3-й": 3, "3-ий": 3, "3-му": 3, "третий": 3, "третьему": 3,
		"4": 4, "4-й": 4, "4-ый": 4, "4-му": 4, "четвертый": 4, "четвёртый": 4,
		"четвертому": 4, "четвёртому": 4,
		"5": 5, "5-й": 5, "5-ый": 5, "5-му": 5, "пятый": 5, "пятому": 5,
		"6": 6, "6-й": 6, "6-ой": 6, "6-му": 6, "шестой": 6, "шестому": 6,
		"7": 7, "7-й": 7, "7-ой": 7, "7-му": 7, "седьмой": 7, "седьмому": 7,
		"8": 8, "8-й": 8, "8-ой": 8, "8-му": 8, "восьмой": 8, "восьмому": 8,
		"9": 9, "9-й": 9, "9-ый": 9, "9-му": 9, "девятый": 9, "девятому": 9,
		"10": 10, "10-й": 10, "10-ый": 10, "10-му": 10,
		"десятый": 10, "десятому": 10,
	}
	ordinal, ok := ordinals[value]
	return ordinal, ok
}

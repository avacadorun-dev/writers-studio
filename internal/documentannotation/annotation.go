package documentannotation

import (
	"errors"
	"sort"
	"strings"

	"writers.studio/internal/domain"
)

var ErrInvalid = errors.New("invalid document annotation")

const MaxNoteRunes = 10000

type TextEdit struct {
	BlockID     string
	Start       int
	End         int
	Expected    string
	Replacement string
}

// RemapAnchor deterministically maps an anchor through exact ranges expressed
// against the same source revision. A partial overlap is deliberately detached:
// guessing a new semantic target would make the audit history unverifiable.
func RemapAnchor(
	anchor domain.AnnotationAnchor,
	edits []TextEdit,
) (domain.AnnotationAnchor, bool, error) {
	ordered := append([]TextEdit(nil), edits...)
	sort.Slice(ordered, func(left, right int) bool {
		if ordered[left].BlockID != ordered[right].BlockID {
			return ordered[left].BlockID < ordered[right].BlockID
		}
		if ordered[left].Start != ordered[right].Start {
			return ordered[left].Start < ordered[right].Start
		}
		return ordered[left].End < ordered[right].End
	})
	previousByBlock := map[string]int{}
	for _, edit := range ordered {
		if strings.TrimSpace(edit.BlockID) == "" || edit.Start < 0 || edit.End < edit.Start ||
			len([]rune(edit.Expected)) != edit.End-edit.Start {
			return domain.AnnotationAnchor{}, false, ErrInvalid
		}
		if previous, exists := previousByBlock[edit.BlockID]; exists && edit.Start < previous {
			return domain.AnnotationAnchor{}, false, ErrInvalid
		}
		previousByBlock[edit.BlockID] = edit.End
	}

	mapped := anchor
	shift := 0
	for _, edit := range ordered {
		if edit.BlockID != anchor.BlockID {
			continue
		}
		delta := len([]rune(edit.Replacement)) - (edit.End - edit.Start)
		switch {
		case edit.End <= anchor.Start:
			shift += delta
		case edit.Start >= anchor.End:
			continue
		case edit.Start == anchor.Start && edit.End == anchor.End:
			if edit.Expected != anchor.ExactText || edit.Replacement == "" {
				return anchor, true, nil
			}
			mapped.Start = anchor.Start + shift
			mapped.End = mapped.Start + len([]rune(edit.Replacement))
			mapped.ExactText = edit.Replacement
			return mapped, false, nil
		default:
			return anchor, true, nil
		}
	}
	mapped.Start += shift
	mapped.End += shift
	return mapped, false, nil
}

func ValidateCreate(
	kind string,
	hasDictionaryTerm bool,
	note string,
	baseRevision int,
	anchor domain.AnnotationAnchor,
) error {
	if kind != "note" && kind != "dictionary_link" {
		return ErrInvalid
	}
	if baseRevision < 1 || anchor.BlockID == "" || anchor.ExactText == "" ||
		anchor.Start < 0 || anchor.End <= anchor.Start {
		return ErrInvalid
	}
	trimmedNote := strings.TrimSpace(note)
	if kind == "note" {
		if hasDictionaryTerm || trimmedNote == "" || len([]rune(trimmedNote)) > MaxNoteRunes {
			return ErrInvalid
		}
		return nil
	}
	if !hasDictionaryTerm || trimmedNote != "" {
		return ErrInvalid
	}
	return nil
}

func NormalizeNote(note string) (string, error) {
	trimmed := strings.TrimSpace(note)
	if trimmed == "" || len([]rune(trimmed)) > MaxNoteRunes {
		return "", ErrInvalid
	}
	return trimmed, nil
}

func ValidStatus(status string) bool {
	return status == "open" || status == "resolved" || status == "needs_attention"
}

func ValidateRequestedStatus(kind string, hasDictionaryTerm bool, status string) error {
	if !ValidStatus(status) {
		return ErrInvalid
	}
	if kind == "dictionary_link" && !hasDictionaryTerm && status == "open" {
		return ErrInvalid
	}
	return nil
}

func StatusAfterMapping(
	currentStatus, kind string,
	hasDictionaryTerm, detached, anchorValid bool,
) string {
	if currentStatus == "resolved" {
		return currentStatus
	}
	if detached || !anchorValid {
		return "needs_attention"
	}
	if currentStatus == "needs_attention" && (kind == "note" || hasDictionaryTerm) {
		return "open"
	}
	return currentStatus
}

func StatusAfterCopy(currentStatus string, mapped, preserveUnmapped bool) string {
	if currentStatus != "resolved" && !mapped && !preserveUnmapped {
		return "needs_attention"
	}
	return currentStatus
}

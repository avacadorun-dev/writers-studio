package assistant

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"writers.studio/internal/editor"
)

var (
	ErrReferenceNotFound     = errors.New("text reference was not found")
	ErrProtectedRangeChanged = errors.New("protected text range was changed")
	ErrMutationOutsideScope  = errors.New("proposal changes text outside the resolved scope")
)

type documentEnvelope struct {
	Type    string          `json:"type"`
	Content []documentBlock `json:"content"`
}

type documentBlock struct {
	Type    string          `json:"type"`
	Attrs   map[string]any  `json:"attrs"`
	Content json.RawMessage `json:"content"`
}

type inlineNode struct {
	Type    string       `json:"type"`
	Text    string       `json:"text"`
	Content []inlineNode `json:"content"`
}

type indexedBlock struct {
	ID      string
	Text    string
	Raw     json.RawMessage
	Ordinal int
}

// NewExplicitResolvedTarget validates stable ranges selected by the model after
// reading project data. It deliberately does not interpret the author's natural
// language: semantic reference resolution belongs to the model, while this layer
// only verifies document identity, revision, block IDs and numeric boundaries.
func NewExplicitResolvedTarget(
	nodeID string,
	revision int,
	content json.RawMessage,
	sourcePhrase string,
	ranges []TextRange,
	protectedRanges []TextRange,
) (ResolvedTarget, error) {
	blocks, err := indexDocumentBlocks(content)
	if err != nil {
		return ResolvedTarget{}, err
	}
	target := ResolvedTarget{
		NodeID: nodeID, BaseRevision: revision, Ranges: ranges,
		ProtectedRanges: protectedRanges, SourcePhrase: strings.TrimSpace(sourcePhrase),
	}
	if err := target.Validate(); err != nil {
		return ResolvedTarget{}, err
	}
	blocksByID := blocksByID(blocks)
	for _, textRange := range append(
		append([]TextRange(nil), ranges...),
		protectedRanges...,
	) {
		block, exists := blocksByID[textRange.BlockID]
		if !exists || textRange.End > len([]rune(block.Text)) {
			return ResolvedTarget{}, ErrReferenceNotFound
		}
	}
	return target, nil
}

// ValidateSelectionAttachment proves that every client-supplied stable anchor
// belongs to the pinned document revision and still names the quoted text. This
// prevents a copied quote or arbitrary node ID from becoming provider context.
func ValidateSelectionAttachment(
	content json.RawMessage,
	selection SelectionAttachment,
) error {
	if err := selection.Validate(); err != nil {
		return err
	}
	blocks, err := indexDocumentBlocks(content)
	if err != nil {
		return err
	}
	byID := blocksByID(blocks)
	var selected []string
	for _, textRange := range selection.Ranges {
		block, exists := byID[textRange.BlockID]
		blockRunes := []rune(block.Text)
		if !exists || textRange.End > len(blockRunes) ||
			string(blockRunes[textRange.Start:textRange.End]) != textRange.ExpectedText {
			return ErrReferenceNotFound
		}
		selected = append(selected, textRange.ExpectedText)
	}
	if strings.TrimSpace(strings.Join(selected, "\n\n")) != strings.TrimSpace(selection.Quote) {
		return ErrReferenceNotFound
	}
	return nil
}

func ValidateProposalScope(
	base json.RawMessage,
	proposed json.RawMessage,
	target ResolvedTarget,
) error {
	if err := target.Validate(); err != nil {
		return err
	}
	baseBlocks, err := indexDocumentBlocks(base)
	if err != nil {
		return err
	}
	proposedBlocks, err := indexDocumentBlocks(proposed)
	if err != nil {
		return err
	}
	baseByID := blocksByID(baseBlocks)
	proposedByID := blocksByID(proposedBlocks)
	protectedByBlock := map[string][]TextRange{}
	for _, protected := range target.ProtectedRanges {
		before, beforeOK := baseByID[protected.BlockID]
		_, afterOK := proposedByID[protected.BlockID]
		if !beforeOK || !afterOK || protected.End > len([]rune(before.Text)) {
			return fmt.Errorf("%w: %s", ErrProtectedRangeChanged, protected.Reason)
		}
		for _, allowed := range target.Ranges {
			if allowed.BlockID == protected.BlockID && rangesOverlap(allowed, protected) {
				return fmt.Errorf("%w: %s", ErrProtectedRangeChanged, protected.Reason)
			}
		}
		protectedByBlock[protected.BlockID] = append(
			protectedByBlock[protected.BlockID], protected,
		)
	}
	allowed := map[string][]TextRange{}
	for _, textRange := range target.Ranges {
		allowed[textRange.BlockID] = append(allowed[textRange.BlockID], textRange)
	}
	for blockID, before := range baseByID {
		after, exists := proposedByID[blockID]
		if !exists {
			if len(allowed[blockID]) == 0 {
				return fmt.Errorf("%w: removed block %s", ErrMutationOutsideScope, blockID)
			}
			continue
		}
		ranges := allowed[blockID]
		if len(ranges) == 0 {
			if !bytes.Equal(normalizeJSON(before.Raw), normalizeJSON(after.Raw)) {
				if protectedRanges := protectedByBlock[blockID]; len(protectedRanges) > 0 &&
					outsideRangesEqual(before.Text, after.Text, protectedRanges) {
					return fmt.Errorf("%w: block %s", ErrProtectedRangeChanged, blockID)
				}
				return fmt.Errorf("%w: block %s", ErrMutationOutsideScope, blockID)
			}
			continue
		}
		if !outsideRangesEqual(before.Text, after.Text, ranges) {
			protectedRanges := protectedByBlock[blockID]
			combinedRanges := append(append([]TextRange(nil), ranges...), protectedRanges...)
			if len(protectedRanges) > 0 &&
				outsideRangesEqual(before.Text, after.Text, combinedRanges) {
				return fmt.Errorf("%w: block %s", ErrProtectedRangeChanged, blockID)
			}
			return fmt.Errorf("%w: block %s", ErrMutationOutsideScope, blockID)
		}
	}
	for blockID := range proposedByID {
		if _, existed := baseByID[blockID]; !existed {
			return fmt.Errorf("%w: added block %s", ErrMutationOutsideScope, blockID)
		}
	}
	return nil
}

func rangesOverlap(left, right TextRange) bool {
	return left.Start < right.End && right.Start < left.End
}

func indexDocumentBlocks(content json.RawMessage) ([]indexedBlock, error) {
	var document editor.Node
	if err := json.Unmarshal(content, &document); err != nil || document.Type != "doc" {
		return nil, errors.New("invalid structured document")
	}
	blocks := make([]indexedBlock, 0, len(document.Content))
	blockIDs := editor.EffectiveTopLevelBlockIDs(document.Content)
	seen := map[string]bool{}
	for index, block := range document.Content {
		blockID := blockIDs[index]
		if blockID == "" || seen[blockID] {
			return nil, errors.New("document block IDs must be present and unique")
		}
		seen[blockID] = true
		raw, err := json.Marshal(block)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, indexedBlock{
			ID: blockID, Text: editorNodeText(block.Content), Raw: raw, Ordinal: index + 1,
		})
	}
	return blocks, nil
}

func editorNodeText(nodes []editor.Node) string {
	var builder strings.Builder
	var walk func([]editor.Node)
	walk = func(current []editor.Node) {
		for _, node := range current {
			builder.WriteString(node.Text)
			walk(node.Content)
		}
	}
	walk(nodes)
	return builder.String()
}

func inlineText(nodes []inlineNode) string {
	var builder strings.Builder
	var walk func([]inlineNode)
	walk = func(current []inlineNode) {
		for _, node := range current {
			builder.WriteString(node.Text)
			walk(node.Content)
		}
	}
	walk(nodes)
	return builder.String()
}

func blocksByID(blocks []indexedBlock) map[string]indexedBlock {
	result := make(map[string]indexedBlock, len(blocks))
	for _, block := range blocks {
		result[block.ID] = block
	}
	return result
}

func normalizeJSON(value json.RawMessage) []byte {
	var decoded any
	if json.Unmarshal(value, &decoded) != nil {
		return value
	}
	normalized, err := json.Marshal(decoded)
	if err != nil {
		return value
	}
	return normalized
}

func outsideRangesEqual(before string, after string, ranges []TextRange) bool {
	beforeRunes := []rune(before)
	afterRunes := []rune(after)
	ordered := append([]TextRange(nil), ranges...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Start < ordered[j].Start })
	if len(ordered) == 0 {
		return before == after
	}

	beforeCursor := 0
	fixedSegments := make([][]rune, 0, len(ordered)+1)
	for _, textRange := range ordered {
		if textRange.Start < beforeCursor || textRange.End > len(beforeRunes) {
			return false
		}
		fixedSegments = append(fixedSegments, beforeRunes[beforeCursor:textRange.Start])
		beforeCursor = textRange.End
	}
	fixedSegments = append(fixedSegments, beforeRunes[beforeCursor:])

	prefix := fixedSegments[0]
	suffix := fixedSegments[len(fixedSegments)-1]
	if len(afterRunes) < len(prefix)+len(suffix) ||
		!runesEqual(afterRunes[:len(prefix)], prefix) ||
		!runesEqual(afterRunes[len(afterRunes)-len(suffix):], suffix) {
		return false
	}
	afterCursor := len(prefix)
	afterLimit := len(afterRunes) - len(suffix)
	for _, fixed := range fixedSegments[1 : len(fixedSegments)-1] {
		match := indexRunes(afterRunes, fixed, afterCursor, afterLimit)
		if match < 0 {
			return false
		}
		afterCursor = match + len(fixed)
	}
	return afterCursor <= afterLimit
}

func indexRunes(haystack, needle []rune, start, limit int) int {
	if start < 0 || limit < start || limit > len(haystack) {
		return -1
	}
	if len(needle) == 0 {
		return start
	}
	for index := start; index+len(needle) <= limit; index++ {
		if runesEqual(haystack[index:index+len(needle)], needle) {
			return index
		}
	}
	return -1
}

func runesEqual(left, right []rune) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

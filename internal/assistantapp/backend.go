package assistantapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"writers.studio/internal/assistant"
	"writers.studio/internal/domain"
	"writers.studio/internal/editor"
	"writers.studio/internal/projectmcp"
	"writers.studio/internal/store"
)

type ProjectBackend struct {
	store   *store.Store
	mu      sync.Mutex
	pending map[string][]assistant.ChangeOperation
}

const (
	maxBatchAddItems       = 100
	maxBatchNodeItems      = 50
	maxLongFormChunks      = 16
	maxLongFormChunkRunes  = 9_000
	maxLongFormChunkWords  = 1_500
	maxLongFormTargetWords = 20_000
	longFormTailRunes      = 800
)

func NewProjectBackend(dataStore *store.Store) *ProjectBackend {
	return &ProjectBackend{
		store: dataStore, pending: make(map[string][]assistant.ChangeOperation),
	}
}

func (b *ProjectBackend) Call(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	name string,
	arguments map[string]any,
) (map[string]any, error) {
	output, err := b.call(ctx, execution, name, arguments)
	if err != nil || !execution.Durable {
		return output, err
	}
	operations := b.PendingOperations(execution.TurnID)
	if len(operations) == 0 {
		return output, nil
	}
	if err := b.store.SaveAssistantTurnDraft(
		ctx, execution.RunID, execution.TurnID, operations, execution.ToolCallCount,
	); err != nil {
		return nil, fmt.Errorf("persist remote MCP turn draft: %w", err)
	}
	return output, nil
}

func (b *ProjectBackend) call(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	name string,
	arguments map[string]any,
) (map[string]any, error) {
	if b == nil || b.store == nil {
		return nil, errors.New("assistant project backend is unavailable")
	}
	policy := assistant.ClassifyTool(name)
	if policy.Authority == assistant.AuthorityNeverExposed {
		return nil, errors.New("tool is not allowed")
	}
	if policy.Authority == assistant.AuthorityRead {
		return b.callRead(ctx, execution, name, arguments)
	}
	if policy.Authority == assistant.AuthorityNativeOnly {
		return b.prepareNativeFlow(ctx, execution, name, arguments)
	}
	if name == "tree.create_nodes_batch" {
		return b.stageBatchNodes(ctx, execution, arguments)
	}
	if name == "document.append_chunk" {
		return b.stageDocumentChunk(ctx, execution, arguments)
	}
	if name == "dictionary.add_batch" || name == "project_memory.add_batch" {
		return b.stageBatchAdds(ctx, execution, name, arguments)
	}
	if name == "annotation.propose_fixes" {
		return b.stageAnnotationFixes(ctx, execution, arguments)
	}
	if name == "document.replace_range" {
		return b.stageDocumentRangeMutation(ctx, execution, arguments)
	}
	if isDocumentContentMutation(name) && b.hasPendingLongFormForTarget(
		execution.TurnID,
		stringArg(arguments, "nodeId"),
	) {
		return nil, errors.New(
			"the document already has a long-form accumulator; continue with document.append_chunk",
		)
	}
	conflict, found, err := b.findMutationConflict(ctx, execution, name, arguments)
	if err != nil {
		return nil, err
	}
	if found {
		return conflict, nil
	}
	operations, err := b.buildMutations(ctx, execution, name, arguments)
	if err != nil {
		return nil, err
	}
	if err := b.appendPendingOperations(execution.TurnID, operations); err != nil {
		return nil, err
	}
	first := operations[0]
	return map[string]any{
		"staged":         true,
		"tool":           name,
		"operationCount": len(operations),
		"groupKey":       first.GroupKey,
		"targetKind":     first.TargetKind,
		"targetId":       first.TargetID,
		"riskLevel":      first.RiskLevel,
		"notice":         "Canonical project state is unchanged until Apply.",
	}, nil
}

func (b *ProjectBackend) stageDocumentRangeMutation(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	arguments map[string]any,
) (map[string]any, error) {
	operations, err := b.buildMutations(ctx, execution, "document.replace_range", arguments)
	if err != nil {
		return nil, err
	}
	if len(operations) != 1 {
		return nil, errors.New("document range mutation must produce one operation")
	}
	merged, err := b.mergeOrAppendDocumentOperations(execution, operations)
	if err != nil {
		return nil, err
	}
	edits, _ := objectSliceArg(arguments, "edits")
	operation := operations[0]
	return map[string]any{
		"staged": true, "merged": merged > 0, "tool": "document.replace_range",
		"operationCount": 1, "editCount": len(edits), "groupKey": operation.GroupKey,
		"targetKind": operation.TargetKind, "targetId": operation.TargetID,
		"riskLevel": operation.RiskLevel,
		"notice":    "Compatible edits for this document are merged into one staged operation. Canonical project state is unchanged until Apply.",
	}, nil
}

func isDocumentContentMutation(name string) bool {
	switch name {
	case "document.insert", "document.replace_range", "document.append",
		"document.apply_marks", "note.update", "annotation.propose_fixes":
		return true
	default:
		return false
	}
}

type annotationFixRequest struct {
	AnnotationID string
	Revision     int
	ExpectedText string
	Replacement  string
	Resolve      bool
}

func (b *ProjectBackend) stageAnnotationFixes(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	arguments map[string]any,
) (map[string]any, error) {
	items, err := objectSliceArg(arguments, "items")
	if err != nil || len(items) < 1 || len(items) > 50 {
		return nil, errors.New("annotation fixes must contain 1 to 50 items")
	}
	requested := make([]annotationFixRequest, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		annotationID := stringArg(item, "annotationId")
		expected, expectedOK := item["expectedText"].(string)
		replacement, replacementOK := item["replacement"].(string)
		if annotationID == "" || seen[annotationID] || intArg(item, "baseRevision") < 1 ||
			!expectedOK || expected == "" || !replacementOK || replacement == "" ||
			replacement == expected {
			return nil, errors.New("annotation fix item is invalid")
		}
		seen[annotationID] = true
		requested = append(requested, annotationFixRequest{
			AnnotationID: annotationID,
			Revision:     intArg(item, "baseRevision"),
			ExpectedText: expected,
			Replacement:  replacement,
			Resolve:      boolArg(item, "resolve"),
		})
	}

	annotations, err := b.store.ListProjectAnnotations(
		ctx, execution.OwnerUserID, execution.ProjectID,
	)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]domain.DocumentAnnotation, len(annotations))
	for _, annotation := range annotations {
		byID[annotation.ID] = annotation
	}
	byNode := make(map[string][]annotationFixRequest)
	nodeOrder := make([]string, 0)
	for _, request := range requested {
		annotation, exists := byID[request.AnnotationID]
		if !exists || annotation.Kind != "note" {
			return nil, errors.New("annotation fix target is not an inline project note")
		}
		if annotation.AnchorStatus != "attached" {
			return nil, errors.New("detached annotation must be reattached manually before fixing")
		}
		if annotation.Status == "resolved" {
			return nil, errors.New("resolved annotation cannot be fixed again")
		}
		if annotation.Revision != request.Revision || annotation.Anchor.ExactText != request.ExpectedText {
			return nil, assistant.ErrStaleChangeSet
		}
		if _, exists := byNode[annotation.NodeID]; !exists {
			nodeOrder = append(nodeOrder, annotation.NodeID)
		}
		byNode[annotation.NodeID] = append(byNode[annotation.NodeID], request)
	}

	operations := make([]assistant.ChangeOperation, 0, len(nodeOrder))
	for _, nodeID := range nodeOrder {
		node, err := b.ownedNode(ctx, execution, nodeID)
		if err != nil {
			return nil, err
		}
		edits := make([]map[string]any, 0, len(byNode[nodeID]))
		fixes := make([]map[string]any, 0, len(byNode[nodeID]))
		ranges := make([]assistant.TextRange, 0, len(byNode[nodeID]))
		for index, request := range byNode[nodeID] {
			annotation := byID[request.AnnotationID]
			if annotation.BaseRevision != node.CurrentRevision ||
				editor.ValidateAnchor(node.Content, annotation.Anchor) != nil {
				return nil, assistant.ErrStaleChangeSet
			}
			edits = append(edits, map[string]any{
				"blockId":      annotation.Anchor.BlockID,
				"start":        annotation.Anchor.Start,
				"end":          annotation.Anchor.End,
				"expectedText": annotation.Anchor.ExactText,
				"replacement":  request.Replacement,
			})
			fixes = append(fixes, map[string]any{
				"annotationId":       annotation.ID,
				"annotationRevision": annotation.Revision,
				"note":               annotation.Note,
				"originalText":       annotation.Anchor.ExactText,
				"replacement":        request.Replacement,
				"resolve":            request.Resolve,
			})
			ranges = append(ranges, assistant.TextRange{
				BlockID: annotation.Anchor.BlockID,
				Start:   annotation.Anchor.Start,
				End:     annotation.Anchor.End,
				Reason:  fmt.Sprintf("inline note %d", index+1),
			})
		}
		operationArguments := map[string]any{
			"nodeId":          node.ID,
			"baseRevision":    node.CurrentRevision,
			"edits":           edits,
			"protectedRanges": []any{},
			"annotationFixes": fixes,
		}
		proposed, err := buildDocumentProposal("document.replace_range", node.Content, operationArguments)
		if err != nil {
			return nil, err
		}
		target, err := assistant.NewExplicitResolvedTarget(
			node.ID, node.CurrentRevision, node.Content, execution.Instruction, ranges, nil,
		)
		if err != nil {
			return nil, err
		}
		if err := assistant.ValidateProposalScope(node.Content, proposed, target); err != nil {
			return nil, err
		}
		baseRevision := node.CurrentRevision
		operations = append(operations, assistant.ChangeOperation{
			ID:               uuid.NewString(),
			GroupKey:         "annotation:" + node.ID,
			Selected:         true,
			ToolName:         "annotation.propose_fixes",
			TargetKind:       "document",
			TargetID:         &node.ID,
			BaseRevision:     &baseRevision,
			Arguments:        mustJSON(operationArguments),
			BeforeSnapshot:   mustJSON(node),
			ProposedSnapshot: proposed,
			Trace: mustJSON(map[string]any{
				"resolvedTarget":  target,
				"annotationFixes": fixes,
				"targetTitle":     node.Title,
			}),
			RiskLevel: assistant.RiskLow,
		})
	}
	if err := validateMutationScope(execution, operations); err != nil {
		return nil, err
	}

	merged, err := b.mergeOrAppendDocumentOperations(execution, operations)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"staged":          true,
		"merged":          merged > 0,
		"tool":            "annotation.propose_fixes",
		"operationCount":  len(operations),
		"annotationCount": len(requested),
		"groupKey":        operations[0].GroupKey,
		"targetKind":      operations[0].TargetKind,
		"riskLevel":       operations[0].RiskLevel,
		"notice":          "Canonical text and annotations are unchanged until Apply.",
	}, nil
}

func (b *ProjectBackend) appendPendingOperations(
	turnID string,
	operations []assistant.ChangeOperation,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	pending := append([]assistant.ChangeOperation(nil), b.pending[turnID]...)
	revisioned := make(map[string]bool)
	for _, operation := range pending {
		if key := operation.RevisionedTargetKey(); key != "" {
			revisioned[key] = true
		}
	}
	for index := range operations {
		if key := operations[index].RevisionedTargetKey(); key != "" {
			if revisioned[key] {
				return errors.New(
					"the same revisioned target already has a staged mutation in this turn",
				)
			}
			revisioned[key] = true
		}
		operations[index].Position = len(pending)
		pending = append(pending, operations[index])
	}
	b.pending[turnID] = pending
	return nil
}

func (b *ProjectBackend) mergeOrAppendDocumentOperations(
	execution projectmcp.ExecutionContext,
	operations []assistant.ChangeOperation,
) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	pending := append([]assistant.ChangeOperation(nil), b.pending[execution.TurnID]...)
	mergedCount := 0
	for _, operation := range operations {
		matched := -1
		if operation.TargetID != nil {
			for index, existing := range pending {
				if existing.TargetKind == "document" && existing.TargetID != nil &&
					*existing.TargetID == *operation.TargetID {
					matched = index
					break
				}
			}
		}
		if matched >= 0 {
			merged, err := mergeDocumentRangeOperations(
				pending[matched], operation, execution.Instruction,
			)
			if err != nil {
				return 0, err
			}
			pending[matched] = merged
			mergedCount++
			continue
		}
		if key := operation.RevisionedTargetKey(); key != "" {
			for _, existing := range pending {
				if existing.RevisionedTargetKey() == key {
					return 0, errors.New(
						"the same revisioned target already has a staged mutation in this turn",
					)
				}
			}
		}
		operation.Position = len(pending)
		pending = append(pending, operation)
	}
	b.pending[execution.TurnID] = pending
	return mergedCount, nil
}

func mergeDocumentRangeOperations(
	existing assistant.ChangeOperation,
	next assistant.ChangeOperation,
	sourcePhrase string,
) (assistant.ChangeOperation, error) {
	if existing.ToolName != next.ToolName ||
		(existing.ToolName != "document.replace_range" &&
			existing.ToolName != "annotation.propose_fixes") {
		return existing, errors.New(
			"the document already has an incompatible staged content mutation in this turn",
		)
	}
	if existing.TargetID == nil || next.TargetID == nil || *existing.TargetID != *next.TargetID ||
		existing.BaseRevision == nil || next.BaseRevision == nil ||
		*existing.BaseRevision != *next.BaseRevision {
		return existing, assistant.ErrStaleChangeSet
	}
	var before domain.DocumentNode
	if err := json.Unmarshal(existing.BeforeSnapshot, &before); err != nil || len(before.Content) == 0 {
		return existing, errors.New("staged document base snapshot is invalid")
	}
	var mergedArguments map[string]any
	var nextArguments map[string]any
	if err := json.Unmarshal(existing.Arguments, &mergedArguments); err != nil {
		return existing, errors.New("staged document arguments are invalid")
	}
	if err := json.Unmarshal(next.Arguments, &nextArguments); err != nil {
		return existing, errors.New("new document arguments are invalid")
	}
	for _, field := range []string{"edits", "protectedRanges", "annotationFixes"} {
		left, err := objectSliceArg(mergedArguments, field)
		if err != nil {
			return existing, err
		}
		right, err := objectSliceArg(nextArguments, field)
		if err != nil {
			return existing, err
		}
		if len(left)+len(right) == 0 {
			continue
		}
		combined := make([]any, 0, len(left)+len(right))
		for _, item := range append(left, right...) {
			combined = append(combined, item)
		}
		mergedArguments[field] = combined
	}
	proposed, err := buildDocumentProposal(
		"document.replace_range", before.Content, mergedArguments,
	)
	if err != nil {
		return existing, err
	}
	ranges, protected, err := explicitMutationRanges("document.replace_range", mergedArguments)
	if err != nil {
		return existing, err
	}
	target, err := assistant.NewExplicitResolvedTarget(
		before.ID, before.CurrentRevision, before.Content, sourcePhrase, ranges, protected,
	)
	if err != nil {
		return existing, err
	}
	if err := assistant.ValidateProposalScope(before.Content, proposed, target); err != nil {
		return existing, err
	}
	existing.Arguments = mustJSON(mergedArguments)
	existing.ProposedSnapshot = proposed
	if existing.ToolName == "annotation.propose_fixes" {
		existing.Trace = mustJSON(map[string]any{
			"resolvedTarget": target, "annotationFixes": mergedArguments["annotationFixes"],
			"targetTitle": before.Title,
		})
	} else {
		existing.Trace = mustJSON(target)
	}
	return existing, nil
}

func (b *ProjectBackend) hasPendingLongFormForTarget(turnID, nodeID string) bool {
	if nodeID == "" {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, operation := range b.pending[turnID] {
		if operation.ToolName == "document.append_chunk" && operation.TargetID != nil &&
			*operation.TargetID == nodeID {
			return true
		}
	}
	return false
}

type longFormChunkState struct {
	ChunkCount       int  `json:"chunkCount"`
	TargetWords      int  `json:"targetWords"`
	AccumulatedWords int  `json:"accumulatedWords"`
	FinalChunk       bool `json:"finalChunk"`
}

func (b *ProjectBackend) stageDocumentChunk(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	arguments map[string]any,
) (map[string]any, error) {
	sequence := intArg(arguments, "sequence")
	targetWords := intArg(arguments, "targetWords")
	text := strings.TrimSpace(stringArg(arguments, "text"))
	chunkWords := len(strings.Fields(text))
	if sequence < 1 || sequence > maxLongFormChunks {
		return nil, fmt.Errorf("long-form sequence must be between 1 and %d", maxLongFormChunks)
	}
	if targetWords < 100 || targetWords > maxLongFormTargetWords {
		return nil, fmt.Errorf("long-form targetWords must be between 100 and %d", maxLongFormTargetWords)
	}
	if text == "" || len([]rune(text)) > maxLongFormChunkRunes || chunkWords > maxLongFormChunkWords {
		return nil, fmt.Errorf(
			"long-form chunk must contain 1 to %d characters and at most %d words",
			maxLongFormChunkRunes,
			maxLongFormChunkWords,
		)
	}
	nodeID := stringArg(arguments, "nodeId")
	baseRevision := intArg(arguments, "baseRevision")
	node, err := b.ownedNode(ctx, execution, nodeID)
	if err != nil {
		return nil, err
	}
	if baseRevision != node.CurrentRevision {
		return nil, assistant.ErrStaleChangeSet
	}
	if err := validateMutationScope(execution, []assistant.ChangeOperation{{
		TargetKind: "document", TargetID: &node.ID,
	}}); err != nil {
		return nil, err
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	operations := b.pending[execution.TurnID]
	accumulatorIndex := -1
	for index := range operations {
		operation := operations[index]
		if operation.TargetID == nil || *operation.TargetID != node.ID {
			continue
		}
		if operation.ToolName != "document.append_chunk" {
			if operation.TargetKind == "document" {
				return nil, errors.New("the document already has another staged content mutation in this turn")
			}
			continue
		}
		accumulatorIndex = index
		break
	}

	var operation assistant.ChangeOperation
	state := longFormChunkState{TargetWords: targetWords}
	if accumulatorIndex < 0 {
		if sequence != 1 {
			return nil, errors.New("the first long-form chunk must use sequence 1")
		}
		enriched := map[string]any{
			"nodeId": node.ID, "baseRevision": baseRevision, "content": text,
		}
		operation, err = b.buildMutation(ctx, execution, "document.append_chunk", enriched)
		if err != nil {
			return nil, err
		}
		operation.Position = len(operations)
		operations = append(operations, operation)
		accumulatorIndex = len(operations) - 1
	} else {
		operation = operations[accumulatorIndex]
		if err := json.Unmarshal(operation.Arguments, &state); err != nil {
			return nil, errors.New("long-form accumulator metadata is invalid")
		}
		if state.FinalChunk {
			return nil, errors.New("the long-form draft was already marked complete")
		}
		if targetWords != state.TargetWords {
			return nil, errors.New("targetWords must stay unchanged across long-form chunks")
		}
		if sequence != state.ChunkCount+1 {
			return nil, fmt.Errorf("expected long-form sequence %d", state.ChunkCount+1)
		}
		proposed, proposalErr := buildDocumentProposal(
			"document.append",
			operation.ProposedSnapshot,
			map[string]any{"content": text},
		)
		if proposalErr != nil {
			return nil, proposalErr
		}
		operation.ProposedSnapshot = proposed
	}

	plainText, err := editor.PlainText(operation.ProposedSnapshot)
	if err != nil {
		return nil, err
	}
	state.ChunkCount = sequence
	state.TargetWords = targetWords
	state.AccumulatedWords += chunkWords
	state.FinalChunk = boolArg(arguments, "finalChunk")
	minimumFinalWords := (targetWords * 80) / 100
	if state.FinalChunk && state.AccumulatedWords < minimumFinalWords {
		return nil, fmt.Errorf(
			"finalChunk requires at least %d accumulated words; currently %d",
			minimumFinalWords,
			state.AccumulatedWords,
		)
	}
	operation.Arguments = mustJSON(state)
	operations[accumulatorIndex] = operation
	b.pending[execution.TurnID] = operations
	remaining := targetWords - state.AccumulatedWords
	if remaining < 0 {
		remaining = 0
	}
	return map[string]any{
		"staged": true, "tool": "document.append_chunk", "operationCount": 1,
		"groupKey": operation.GroupKey, "targetKind": operation.TargetKind,
		"targetId": operation.TargetID, "riskLevel": operation.RiskLevel,
		"chunkSequence": sequence, "nextSequence": sequence + 1,
		"chunkWords": chunkWords, "accumulatedWords": state.AccumulatedWords,
		"targetWords": targetWords, "remainingTargetWords": remaining,
		"finalChunk":       state.FinalChunk,
		"continuationTail": trailingRunes(plainText, longFormTailRunes),
		"notice":           "The chunk was merged into one staged long-form document operation. Canonical state is unchanged until Apply.",
	}, nil
}

func trailingRunes(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[len(runes)-limit:])
}

func (b *ProjectBackend) stageBatchNodes(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	arguments map[string]any,
) (map[string]any, error) {
	items, err := objectSliceArg(arguments, "items")
	if err != nil {
		return nil, fmt.Errorf("invalid node batch items: %w", err)
	}
	if len(items) < 1 || len(items) > maxBatchNodeItems {
		return nil, fmt.Errorf("node batch requires between 1 and %d items", maxBatchNodeItems)
	}
	parentID := nullableStringArg(arguments, "parentId")
	afterID := nullableStringArg(arguments, "positionAfterId")
	positions, err := b.batchNodePositions(
		ctx, execution, parentID, afterID, len(items),
	)
	if err != nil {
		return nil, err
	}

	operations := make([]assistant.ChangeOperation, 0, len(items))
	for index, item := range items {
		enriched := make(map[string]any, len(item)+2)
		for key, value := range item {
			enriched[key] = value
		}
		enriched["parentId"] = nil
		if parentID != nil {
			enriched["parentId"] = *parentID
		}
		enriched["positionAfterId"] = nil
		if afterID != nil {
			enriched["positionAfterId"] = *afterID
		}
		operation, buildErr := b.buildMutation(
			ctx, execution, "tree.create_node", enriched,
		)
		if buildErr != nil {
			return nil, fmt.Errorf("node batch item %d: %w", index, buildErr)
		}
		var proposal nodeProposal
		if err := json.Unmarshal(operation.ProposedSnapshot, &proposal); err != nil {
			return nil, fmt.Errorf("node batch item %d proposal: %w", index, err)
		}
		proposal.Position = positions[index]
		operation.ProposedSnapshot = mustJSON(proposal)
		operation.Arguments = mustJSON(enriched)
		operations = append(operations, operation)
	}
	if err := validateMutationScope(execution, operations); err != nil {
		return nil, err
	}

	b.mu.Lock()
	startPosition := len(b.pending[execution.TurnID])
	for index := range operations {
		operations[index].Position = startPosition + index
	}
	b.pending[execution.TurnID] = append(b.pending[execution.TurnID], operations...)
	b.mu.Unlock()
	return map[string]any{
		"staged": true, "tool": "tree.create_nodes_batch",
		"requestedCount": len(items), "operationCount": len(operations),
		"groupKey": operations[0].GroupKey, "targetKind": operations[0].TargetKind,
		"riskLevel": operations[0].RiskLevel,
		"notice":    "Canonical project state is unchanged until Apply. Nodes already include their complete initial content and will be inserted in array order.",
	}, nil
}

func (b *ProjectBackend) batchNodePositions(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	parentID *string,
	afterID *string,
	count int,
) ([]float64, error) {
	nodes, err := b.store.ListDocumentNodes(
		ctx, execution.OwnerUserID, execution.ProjectID, false,
	)
	if err != nil {
		return nil, err
	}
	siblings := make([]domain.DocumentNode, 0)
	for _, node := range nodes {
		if sameOptionalString(node.ParentID, parentID) {
			siblings = append(siblings, node)
		}
	}
	sort.Slice(siblings, func(left, right int) bool {
		if siblings[left].Position == siblings[right].Position {
			return siblings[left].ID < siblings[right].ID
		}
		return siblings[left].Position < siblings[right].Position
	})
	positions := make([]float64, count)
	if afterID == nil {
		start := 0.0
		if len(siblings) > 0 {
			start = siblings[len(siblings)-1].Position
		}
		for index := range positions {
			positions[index] = start + 1000*float64(index+1)
		}
		return positions, nil
	}

	anchorIndex := -1
	for index, sibling := range siblings {
		if sibling.ID == *afterID {
			anchorIndex = index
			break
		}
	}
	if anchorIndex < 0 {
		return nil, errors.New("position anchor was not found under the selected parent")
	}
	left := siblings[anchorIndex].Position
	step := 1000.0
	if anchorIndex+1 < len(siblings) {
		right := siblings[anchorIndex+1].Position
		step = (right - left) / float64(count+1)
		if step <= 0 {
			return nil, errors.New("selected node positions do not leave an ordered insertion range")
		}
	}
	for index := range positions {
		positions[index] = left + step*float64(index+1)
	}
	return positions, nil
}

func (b *ProjectBackend) stageBatchAdds(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	name string,
	arguments map[string]any,
) (map[string]any, error) {
	items, err := objectSliceArg(arguments, "items")
	if err != nil {
		return nil, fmt.Errorf("invalid batch items: %w", err)
	}
	if len(items) < 2 || len(items) > maxBatchAddItems {
		return nil, fmt.Errorf("batch add requires between 2 and %d items", maxBatchAddItems)
	}

	var singleTool string
	var existingTerms []domain.DictionaryTerm
	var existingMemory []assistant.MemoryRecord
	switch name {
	case "dictionary.add_batch":
		singleTool = "dictionary.add"
		existingTerms, err = b.store.ListDictionaryTerms(
			ctx, execution.OwnerUserID, execution.ProjectID,
		)
	case "project_memory.add_batch":
		singleTool = "project_memory.add"
		existingMemory, err = b.store.ListProjectMemory(
			ctx, execution.OwnerUserID, execution.ProjectID, true,
		)
	default:
		return nil, errors.New("batch add tool is not supported")
	}
	if err != nil {
		return nil, err
	}

	operations := make([]assistant.ChangeOperation, 0, len(items))
	conflicts := make([]map[string]any, 0)
	seenDictionary := make([][]string, 0, len(items))
	seenDictionaryIndices := make([]int, 0, len(items))
	seenMemory := make(map[string]int, len(items))
	for index, item := range items {
		if singleTool == "dictionary.add" {
			values := append(
				[]string{stringArg(item, "canonical")},
				stringSliceArg(item, "variants")...,
			)
			if strings.TrimSpace(values[0]) == "" {
				return nil, fmt.Errorf("dictionary batch item %d has an empty canonical name", index)
			}
			var candidates []domain.DictionaryTerm
			for _, term := range existingTerms {
				if normalizedValuesOverlap(values, append([]string{term.Canonical}, term.Variants...)) {
					candidates = append(candidates, term)
				}
			}
			if len(candidates) > 0 {
				conflicts = append(conflicts, map[string]any{
					"index": index, "kind": "dictionary_duplicate",
					"candidates":  candidates,
					"nextActions": []string{"dictionary.update", "dictionary.set_active"},
					"notice":      "A matching canonical name or variant already exists.",
				})
				continue
			}
			batchDuplicate := -1
			for previous, seen := range seenDictionary {
				if normalizedValuesOverlap(values, seen) {
					batchDuplicate = seenDictionaryIndices[previous]
					break
				}
			}
			if batchDuplicate >= 0 {
				conflicts = append(conflicts, map[string]any{
					"index": index, "kind": "dictionary_batch_duplicate",
					"batchCandidateIndex": batchDuplicate,
					"notice":              "This item overlaps another term in the same batch.",
				})
				continue
			}
			seenDictionary = append(seenDictionary, values)
			seenDictionaryIndices = append(seenDictionaryIndices, index)
		} else {
			statement := strings.TrimSpace(stringArg(item, "statement"))
			if statement == "" {
				return nil, fmt.Errorf("project memory batch item %d has an empty statement", index)
			}
			var candidates []assistant.MemoryRecord
			for _, record := range existingMemory {
				if strings.EqualFold(strings.TrimSpace(record.Statement), statement) {
					candidates = append(candidates, record)
				}
			}
			if len(candidates) > 0 {
				conflicts = append(conflicts, map[string]any{
					"index": index, "kind": "memory_duplicate", "candidates": candidates,
					"nextActions": []string{
						"project_memory.update", "project_memory.set_active",
						"project_memory.set_always_relevant",
					},
					"notice": "An identical project-memory statement already exists.",
				})
				continue
			}
			normalized := strings.ToLower(statement)
			if previous, exists := seenMemory[normalized]; exists {
				conflicts = append(conflicts, map[string]any{
					"index": index, "kind": "memory_batch_duplicate",
					"batchCandidateIndex": previous,
					"notice":              "This statement is duplicated in the same batch.",
				})
				continue
			}
			seenMemory[normalized] = index
		}

		operation, buildErr := b.buildMutation(ctx, execution, singleTool, item)
		if buildErr != nil {
			return nil, fmt.Errorf("batch item %d: %w", index, buildErr)
		}
		operations = append(operations, operation)
	}
	if err := validateMutationScope(execution, operations); err != nil {
		return nil, err
	}
	if len(operations) == 0 {
		return map[string]any{
			"staged": false, "tool": name, "requestedCount": len(items),
			"operationCount": 0, "conflictCount": len(conflicts), "conflicts": conflicts,
			"notice": "No new operations were staged because every item conflicts with an existing or repeated item.",
		}, nil
	}

	b.mu.Lock()
	startPosition := len(b.pending[execution.TurnID])
	for index := range operations {
		operations[index].Position = startPosition + index
	}
	b.pending[execution.TurnID] = append(b.pending[execution.TurnID], operations...)
	b.mu.Unlock()
	first := operations[0]
	return map[string]any{
		"staged": true, "tool": name, "requestedCount": len(items),
		"operationCount": len(operations), "conflictCount": len(conflicts),
		"conflicts": conflicts, "groupKey": first.GroupKey,
		"targetKind": first.TargetKind, "riskLevel": first.RiskLevel,
		"notice": "Canonical project state is unchanged until Apply. Each batch item is independently reviewable.",
	}, nil
}

func (b *ProjectBackend) prepareNativeFlow(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	name string,
	arguments map[string]any,
) (map[string]any, error) {
	var kind string
	var destination map[string]any
	switch name {
	case "tree.prepare_archive":
		nodeID := stringArg(arguments, "nodeId")
		node, err := b.store.GetDocumentNode(ctx, execution.OwnerUserID, nodeID)
		if err != nil {
			return nil, err
		}
		if node.ProjectID != execution.ProjectID {
			return nil, pgx.ErrNoRows
		}
		kind = "archive_node"
		destination = map[string]any{
			"type": "project_node", "projectId": execution.ProjectID, "nodeId": node.ID,
		}
	case "version.prepare_restore":
		versionID := stringArg(arguments, "versionId")
		version, err := b.store.GetNamedVersion(ctx, execution.OwnerUserID, versionID)
		if err != nil {
			return nil, err
		}
		node, err := b.store.GetDocumentNode(ctx, execution.OwnerUserID, version.NodeID)
		if err != nil {
			return nil, err
		}
		if node.ProjectID != execution.ProjectID {
			return nil, pgx.ErrNoRows
		}
		expectedRevision := intArg(arguments, "expectedCurrentRevision")
		if expectedRevision != node.CurrentRevision {
			return nil, assistant.ErrStaleChangeSet
		}
		kind = "restore_version"
		destination = map[string]any{
			"type": "node_versions", "nodeId": node.ID,
			"versionId": version.ID, "expectedCurrentRevision": expectedRevision,
		}
	case "transcript.prepare_retranscribe":
		sessionID := stringArg(arguments, "sessionId")
		session, err := b.store.GetDictationSession(ctx, execution.OwnerUserID, sessionID)
		if err != nil {
			return nil, err
		}
		if session.ProjectID != execution.ProjectID {
			return nil, pgx.ErrNoRows
		}
		kind = "retranscribe"
		destination = map[string]any{
			"type": "dictation_review", "sessionId": session.ID,
		}
	case "export.prepare":
		format := stringArg(arguments, "format")
		switch format {
		case "docx", "epub", "pdf", "markdown", "txt":
		default:
			return nil, errors.New("export format is not supported")
		}
		if _, err := b.exportPreview(ctx, execution, arguments); err != nil {
			return nil, err
		}
		kind = "export"
		destination = map[string]any{
			"type": "project_export", "projectId": execution.ProjectID,
			"format": format, "nodeIds": stringSliceArg(arguments, "nodeIds"),
			"chapterTitles": boolArg(arguments, "chapterTitles"),
		}
	default:
		return nil, errors.New("native flow is not supported")
	}
	return map[string]any{
		"prepared": true,
		"nativeFlow": map[string]any{
			"kind":        kind,
			"destination": destination,
			"notice":      "Canonical project state is unchanged. Confirm in the native Writers Studio flow.",
		},
	}, nil
}

func (b *ProjectBackend) findMutationConflict(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	name string,
	arguments map[string]any,
) (map[string]any, bool, error) {
	switch name {
	case "dictionary.add":
		values := append(
			[]string{stringArg(arguments, "canonical")},
			stringSliceArg(arguments, "variants")...,
		)
		terms, err := b.store.ListDictionaryTerms(
			ctx,
			execution.OwnerUserID,
			execution.ProjectID,
		)
		if err != nil {
			return nil, false, err
		}
		var candidates []domain.DictionaryTerm
		for _, term := range terms {
			existing := append([]string{term.Canonical}, term.Variants...)
			if normalizedValuesOverlap(values, existing) {
				candidates = append(candidates, term)
			}
		}
		if len(candidates) == 0 {
			return nil, false, nil
		}
		return map[string]any{
			"staged": false, "conflict": true, "kind": "dictionary_duplicate",
			"candidates":  candidates,
			"nextActions": []string{"dictionary.update", "dictionary.set_active"},
			"notice":      "A matching canonical name or variant already exists. Review the candidate and stage an update/merge instead of creating a duplicate.",
		}, true, nil
	case "project_memory.add":
		statement := strings.TrimSpace(stringArg(arguments, "statement"))
		if statement == "" {
			return nil, false, nil
		}
		records, err := b.store.ListProjectMemory(
			ctx,
			execution.OwnerUserID,
			execution.ProjectID,
			true,
		)
		if err != nil {
			return nil, false, err
		}
		var candidates []assistant.MemoryRecord
		for _, record := range records {
			if strings.EqualFold(strings.TrimSpace(record.Statement), statement) {
				candidates = append(candidates, record)
			}
		}
		if len(candidates) == 0 {
			return nil, false, nil
		}
		return map[string]any{
			"staged": false, "conflict": true, "kind": "memory_duplicate",
			"candidates": candidates,
			"nextActions": []string{
				"project_memory.update",
				"project_memory.set_active",
				"project_memory.set_always_relevant",
			},
			"notice": "An identical project-memory statement already exists. Review it and stage an update/reactivation instead of creating a duplicate.",
		}, true, nil
	default:
		return nil, false, nil
	}
}

func normalizedValuesOverlap(left, right []string) bool {
	values := make(map[string]struct{}, len(left))
	for _, value := range left {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			values[value] = struct{}{}
		}
	}
	for _, value := range right {
		value = strings.ToLower(strings.TrimSpace(value))
		if _, exists := values[value]; value != "" && exists {
			return true
		}
	}
	return false
}

func (b *ProjectBackend) buildMutations(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	name string,
	arguments map[string]any,
) ([]assistant.ChangeOperation, error) {
	if name == "document.prepare_replace_all" {
		operations, err := b.buildReplaceAllMutations(ctx, execution, arguments)
		if err != nil {
			return nil, err
		}
		if err := validateMutationScope(execution, operations); err != nil {
			return nil, err
		}
		return operations, nil
	}
	operation, err := b.buildMutation(ctx, execution, name, arguments)
	if err != nil {
		return nil, err
	}
	if err := validateMutationScope(execution, []assistant.ChangeOperation{operation}); err != nil {
		return nil, err
	}
	return []assistant.ChangeOperation{operation}, nil
}

func validateMutationScope(
	execution projectmcp.ExecutionContext,
	operations []assistant.ChangeOperation,
) error {
	if execution.TargetNodeID == nil {
		return nil
	}
	for _, operation := range operations {
		switch operation.TargetKind {
		case "document", "node":
			if operation.TargetID == nil || *operation.TargetID != *execution.TargetNodeID {
				return fmt.Errorf(
					"%w: node-scoped run may only change target node %s",
					assistant.ErrMutationOutsideScope,
					*execution.TargetNodeID,
				)
			}
		case "version":
			var proposal struct {
				NodeID string `json:"nodeId"`
			}
			if json.Unmarshal(operation.ProposedSnapshot, &proposal) != nil ||
				proposal.NodeID != *execution.TargetNodeID {
				return fmt.Errorf(
					"%w: node-scoped run may only version target node %s",
					assistant.ErrMutationOutsideScope,
					*execution.TargetNodeID,
				)
			}
		}
	}
	return nil
}

func (b *ProjectBackend) buildReplaceAllMutations(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	arguments map[string]any,
) ([]assistant.ChangeOperation, error) {
	query := stringArg(arguments, "query")
	if query == "" {
		return nil, errors.New("replacement query must not be empty")
	}
	replacement := stringArg(arguments, "replacement")
	requestedIDs := stringSliceArg(arguments, "nodeIds")
	if len(requestedIDs) > 100 {
		return nil, errors.New("project-wide replacement is limited to 100 documents")
	}

	nodes := make([]domain.DocumentNode, 0, len(requestedIDs))
	if len(requestedIDs) == 0 {
		projectNodes, err := b.store.ListDocumentNodes(
			ctx,
			execution.OwnerUserID,
			execution.ProjectID,
			false,
		)
		if err != nil {
			return nil, err
		}
		for _, node := range projectNodes {
			if node.CurrentRevision > 0 && len(node.Content) > 0 {
				nodes = append(nodes, node)
			}
		}
	} else {
		seen := make(map[string]bool, len(requestedIDs))
		for _, nodeID := range requestedIDs {
			if seen[nodeID] {
				continue
			}
			seen[nodeID] = true
			node, err := b.ownedNode(ctx, execution, nodeID)
			if err != nil {
				return nil, err
			}
			if node.CurrentRevision == 0 || len(node.Content) == 0 {
				return nil, fmt.Errorf("node %q does not contain an editable document", nodeID)
			}
			nodes = append(nodes, node)
		}
	}

	operations := make([]assistant.ChangeOperation, 0, len(nodes))
	for _, node := range nodes {
		proposed, count, err := editor.ReplaceText(node.Content, query, replacement)
		if err != nil {
			return nil, err
		}
		if count == 0 {
			continue
		}
		baseRevision := node.CurrentRevision
		targetID := node.ID
		operations = append(operations, assistant.ChangeOperation{
			ID:               uuid.NewString(),
			GroupKey:         "document:" + node.ID,
			Selected:         true,
			ToolName:         "document.prepare_replace_all",
			TargetKind:       "document",
			TargetID:         &targetID,
			BaseRevision:     &baseRevision,
			Arguments:        mustJSON(arguments),
			BeforeSnapshot:   mustJSON(node),
			ProposedSnapshot: proposed,
			Trace: mustJSON(map[string]any{
				"targetNodeId": node.ID, "targetTitle": node.Title,
				"baseRevision": node.CurrentRevision, "query": query,
				"replacement": replacement, "matchCount": count,
			}),
			RiskLevel: assistant.RiskHigh,
		})
	}
	if len(operations) == 0 {
		return nil, errors.New("no selected project document contains the replacement query")
	}
	return operations, nil
}

func (b *ProjectBackend) FinalizeChangeSet(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	title string,
	summary string,
) (*assistant.ChangeSet, error) {
	b.mu.Lock()
	operations := append([]assistant.ChangeOperation(nil), b.pending[execution.TurnID]...)
	b.mu.Unlock()
	if len(operations) == 0 {
		if execution.Durable {
			_ = b.store.DeleteAssistantTurnDraft(ctx, execution.TurnID)
		}
		return nil, nil
	}
	fingerprint, err := b.store.ProjectFingerprint(
		ctx,
		execution.OwnerUserID,
		execution.ProjectID,
	)
	if err != nil {
		return nil, err
	}
	risk := assistant.RiskLow
	for _, operation := range operations {
		if operation.RiskLevel == assistant.RiskHigh {
			risk = assistant.RiskHigh
			break
		}
		if operation.RiskLevel == assistant.RiskMedium {
			risk = assistant.RiskMedium
		}
	}
	staged := assistant.ChangeSet{
		RunID: execution.RunID, TurnID: execution.TurnID,
		Title: strings.TrimSpace(title), Summary: summarizeChangeOperations(operations, summary),
		RiskLevel: risk, BaseFingerprint: fingerprint, Operations: operations,
	}
	var changeSet assistant.ChangeSet
	if execution.Durable {
		changeSet, err = b.store.StageAssistantChangeSetFromDraft(ctx, staged, execution.TurnID)
	} else {
		changeSet, err = b.store.StageAssistantChangeSet(ctx, staged)
	}
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	delete(b.pending, execution.TurnID)
	b.mu.Unlock()
	return &changeSet, nil
}

func summarizeChangeOperations(
	operations []assistant.ChangeOperation,
	fallback string,
) string {
	labels := map[string]string{
		"document":   "текст и структура",
		"dictionary": "словарь",
		"memory":     "память проекта",
		"guide":      "руководство ассистента",
		"version":    "версия",
	}
	counts := map[string]int{}
	var order []string
	for _, operation := range operations {
		group := operation.GroupKey
		if strings.HasPrefix(group, "document:") {
			group = "document"
		}
		if _, exists := counts[group]; !exists {
			order = append(order, group)
		}
		counts[group]++
	}
	var parts []string
	for _, group := range order {
		label := labels[group]
		if label == "" {
			label = group
		}
		parts = append(parts, fmt.Sprintf("%s · %d", label, counts[group]))
	}
	if len(parts) > 0 {
		return fmt.Sprintf(
			"Подготовлено операций: %d. %s. Ничего не применено до подтверждения.",
			len(operations),
			strings.Join(parts, "; "),
		)
	}
	return strings.TrimSpace(fallback)
}

func (b *ProjectBackend) DiscardPending(turnID string) {
	b.mu.Lock()
	delete(b.pending, turnID)
	b.mu.Unlock()
}

func (b *ProjectBackend) DiscardPendingPersistent(ctx context.Context, turnID string) error {
	b.DiscardPending(turnID)
	return b.store.DeleteAssistantTurnDraft(ctx, turnID)
}

func (b *ProjectBackend) PendingOperations(turnID string) []assistant.ChangeOperation {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]assistant.ChangeOperation(nil), b.pending[turnID]...)
}

func (b *ProjectBackend) RestorePending(
	ctx context.Context,
	turnID string,
) (int, error) {
	operations, callCount, err := b.store.LoadAssistantTurnDraft(ctx, turnID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	b.mu.Lock()
	b.pending[turnID] = append([]assistant.ChangeOperation(nil), operations...)
	b.mu.Unlock()
	return callCount, nil
}

// StageRemoteHighRisk turns the two former native-only document actions into
// normal durable change operations for remote MCP. The built-in catalog keeps
// its existing prepare-only tools and behavior.
func (b *ProjectBackend) StageRemoteHighRisk(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	name string,
	arguments map[string]any,
) (map[string]any, error) {
	var operation assistant.ChangeOperation
	switch name {
	case "tree.archive":
		node, err := b.ownedNode(ctx, execution, stringArg(arguments, "nodeId"))
		if err != nil {
			return nil, err
		}
		targetID := node.ID
		operation = assistant.ChangeOperation{
			ID: uuid.NewString(), GroupKey: "archive:" + node.ID, Selected: true,
			ToolName: name, TargetKind: "node", TargetID: &targetID,
			Arguments: mustJSON(arguments), BeforeSnapshot: mustJSON(node),
			ProposedSnapshot: mustJSON(map[string]any{
				"nodeId": node.ID, "title": node.Title, "trashed": true,
			}),
			Trace:     mustJSON(map[string]any{"nodeId": node.ID, "title": node.Title}),
			RiskLevel: assistant.RiskHigh,
		}
	case "version.restore":
		version, current, versionContent, err := b.store.GetNamedVersionDocument(
			ctx, execution.OwnerUserID, stringArg(arguments, "versionId"),
		)
		if err != nil {
			return nil, err
		}
		if current.ProjectID != execution.ProjectID {
			return nil, pgx.ErrNoRows
		}
		expected := intArg(arguments, "expectedCurrentRevision")
		if current.CurrentRevision != expected {
			return nil, assistant.ErrStaleChangeSet
		}
		targetID := current.ID
		baseRevision := current.CurrentRevision
		operation = assistant.ChangeOperation{
			ID: uuid.NewString(), GroupKey: "document:" + current.ID, Selected: true,
			ToolName: name, TargetKind: "document", TargetID: &targetID,
			BaseRevision: &baseRevision, Arguments: mustJSON(arguments),
			BeforeSnapshot: mustJSON(current), ProposedSnapshot: versionContent,
			Trace: mustJSON(map[string]any{
				"versionId": version.ID, "versionRevision": version.Revision,
				"baseRevision": current.CurrentRevision,
			}),
			RiskLevel: assistant.RiskHigh,
		}
	default:
		return nil, errors.New("unsupported remote high-risk operation")
	}
	if err := b.appendPendingOperations(execution.TurnID, []assistant.ChangeOperation{operation}); err != nil {
		return nil, err
	}
	if execution.Durable {
		if err := b.store.SaveAssistantTurnDraft(
			ctx, execution.RunID, execution.TurnID,
			b.PendingOperations(execution.TurnID), execution.ToolCallCount,
		); err != nil {
			return nil, err
		}
	}
	return map[string]any{
		"staged": true, "tool": name, "operationCount": 1,
		"groupKey": operation.GroupKey, "targetKind": operation.TargetKind,
		"targetId": operation.TargetID, "riskLevel": operation.RiskLevel,
		"notice": "Canonical project state is unchanged until Apply.",
	}, nil
}

func (b *ProjectBackend) PendingCount(turnID string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pending[turnID])
}

func (b *ProjectBackend) PendingLongFormIncomplete(turnID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, operation := range b.pending[turnID] {
		if operation.ToolName != "document.append_chunk" {
			continue
		}
		var state longFormChunkState
		if json.Unmarshal(operation.Arguments, &state) != nil || !state.FinalChunk {
			return true
		}
	}
	return false
}

// PendingDocumentPreview returns the newest real staged document proposal for
// the live UI. It is intentionally read-only and remains outside canonical
// revisions and editor history until the normal change-set Apply transaction.
func (b *ProjectBackend) PendingDocumentPreview(turnID string) (map[string]any, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	operations := b.pending[turnID]
	for index := len(operations) - 1; index >= 0; index-- {
		operation := operations[index]
		if operation.TargetKind != "document" || operation.TargetID == nil {
			continue
		}
		return map[string]any{
			"operationId":      operation.ID,
			"targetNodeId":     *operation.TargetID,
			"baseRevision":     operation.BaseRevision,
			"toolName":         operation.ToolName,
			"arguments":        operation.Arguments,
			"beforeSnapshot":   operation.BeforeSnapshot,
			"proposedSnapshot": operation.ProposedSnapshot,
			"trace":            operation.Trace,
			"canonical":        false,
		}, true
	}
	return nil, false
}

func (b *ProjectBackend) callRead(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	name string,
	arguments map[string]any,
) (map[string]any, error) {
	switch name {
	case "assistant.begin_turn":
		mode := stringArg(arguments, "mode")
		if mode != "answer" && mode != "change" && mode != "clarify" {
			return nil, errors.New("assistant turn mode is invalid")
		}
		return map[string]any{
			"accepted": true, "mode": mode,
			"notice": "Natural-language semantics were declared by the model, not inferred by the server.",
		}, nil
	case "assistant.get_recent_change_set":
		changeSet, err := b.store.GetLatestAssistantChangeSet(
			ctx,
			execution.OwnerUserID,
			execution.RunID,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return map[string]any{"exists": false}, nil
		}
		if err != nil {
			return nil, err
		}
		return map[string]any{"exists": true, "changeSet": changeSet}, nil
	case "project.get_overview":
		workspace, err := b.store.GetProject(ctx, execution.OwnerUserID, execution.ProjectID)
		if err != nil {
			return nil, err
		}
		activeNodes := 0
		totalWords := 0
		for _, node := range workspace.Nodes {
			if node.TrashedAt == nil {
				activeNodes++
				totalWords += len(strings.Fields(node.PlainText))
			}
		}
		return map[string]any{
			"project": workspace.Project, "activeNodeCount": activeNodes,
			"wordCount": totalWords, "targetNodeId": execution.TargetNodeID,
		}, nil
	case "project.get_structure":
		structure, revision, err := b.store.GetProjectStructure(
			ctx, execution.OwnerUserID, execution.ProjectID,
		)
		if err != nil {
			return nil, err
		}
		return map[string]any{"revision": revision, "structure": structure}, nil
	case "project.list_recent":
		nodes, err := b.store.ListDocumentNodes(
			ctx, execution.OwnerUserID, execution.ProjectID, false,
		)
		if err != nil {
			return nil, err
		}
		sort.Slice(nodes, func(i, j int) bool { return nodes[i].UpdatedAt.After(nodes[j].UpdatedAt) })
		limit := boundedInt(arguments["limit"], 10, 1, 50)
		if len(nodes) > limit {
			nodes = nodes[:limit]
		}
		return map[string]any{"nodes": nodes}, nil
	case "project.search":
		results, err := b.searchProjectContext(ctx, execution, stringArg(arguments, "query"))
		if err != nil {
			return nil, err
		}
		return map[string]any{"results": results}, nil
	case "project.get_activity":
		return b.projectActivity(
			ctx,
			execution,
			boundedInt(arguments["limit"], 30, 1, 100),
		)
	case "tree.list":
		nodes, err := b.store.ListDocumentNodes(
			ctx, execution.OwnerUserID, execution.ProjectID, false,
		)
		if err != nil {
			return nil, err
		}
		for index := range nodes {
			nodes[index].Content = nil
			nodes[index].PlainText = ""
		}
		return map[string]any{"nodes": nodes}, nil
	case "tree.get_node":
		node, err := b.ownedNode(ctx, execution, stringArg(arguments, "nodeId"))
		if err != nil {
			return nil, err
		}
		node.Content = nil
		node.PlainText = ""
		return toMap("node", node)
	case "document.get":
		node, err := b.ownedNode(ctx, execution, stringArg(arguments, "nodeId"))
		if err != nil {
			return nil, err
		}
		return toMap("document", node)
	case "document.find_exact":
		node, err := b.ownedNode(ctx, execution, stringArg(arguments, "nodeId"))
		if err != nil {
			return nil, err
		}
		node, err = b.documentAtRevision(
			ctx,
			execution.OwnerUserID,
			node,
			intArg(arguments, "revision"),
		)
		if err != nil {
			return nil, err
		}
		return findExactDocumentAnchors(
			node,
			stringArg(arguments, "text"),
			nullableStringArg(arguments, "blockId"),
		)
	case "document.find_exact_batch":
		node, err := b.ownedNode(ctx, execution, stringArg(arguments, "nodeId"))
		if err != nil {
			return nil, err
		}
		node, err = b.documentAtRevision(
			ctx,
			execution.OwnerUserID,
			node,
			intArg(arguments, "revision"),
		)
		if err != nil {
			return nil, err
		}
		return findExactDocumentAnchorsBatch(node, arguments["queries"])
	case "document.get_range":
		node, err := b.ownedNode(ctx, execution, stringArg(arguments, "nodeId"))
		if err != nil {
			return nil, err
		}
		revision := intArg(arguments, "revision")
		node, err = b.documentAtRevision(ctx, execution.OwnerUserID, node, revision)
		if err != nil {
			return nil, err
		}
		runes := []rune(node.PlainText)
		start := intArg(arguments, "start")
		end := intArg(arguments, "end")
		if start < 0 || end < start || end > len(runes) {
			return nil, errors.New("document range is out of bounds")
		}
		return map[string]any{
			"nodeId": node.ID, "revision": revision, "start": start, "end": end,
			"text": string(runes[start:end]),
		}, nil
	case "document.list_references":
		node, err := b.ownedNode(ctx, execution, stringArg(arguments, "nodeId"))
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"nodeId": node.ID, "revision": node.CurrentRevision,
			"references": documentReferences(node.Content),
		}, nil
	case "annotation.list":
		annotations, err := b.store.ListProjectAnnotations(
			ctx, execution.OwnerUserID, execution.ProjectID,
		)
		if err != nil {
			return nil, err
		}
		kind := stringArg(arguments, "kind")
		status := stringArg(arguments, "status")
		anchorStatus := stringArg(arguments, "anchorStatus")
		nodeID := nullableStringArg(arguments, "nodeId")
		limit := boundedInt(arguments["limit"], 100, 1, 500)
		filtered := make([]domain.DocumentAnnotation, 0, min(limit, len(annotations)))
		for _, annotation := range annotations {
			if kind != "all" && annotation.Kind != kind ||
				status != "all" && annotation.Status != status ||
				anchorStatus != "all" && annotation.AnchorStatus != anchorStatus ||
				nodeID != nil && annotation.NodeID != *nodeID {
				continue
			}
			filtered = append(filtered, annotation)
			if len(filtered) == limit {
				break
			}
		}
		return map[string]any{"annotations": filtered}, nil
	case "annotation.get":
		annotation, err := b.store.GetDocumentAnnotation(
			ctx, execution.OwnerUserID, stringArg(arguments, "annotationId"),
		)
		if err != nil || annotation.ProjectID != execution.ProjectID {
			if err == nil {
				err = pgx.ErrNoRows
			}
			return nil, err
		}
		history, err := b.store.ListDocumentAnnotationHistory(
			ctx, execution.OwnerUserID, annotation.ID,
		)
		if err != nil {
			return nil, err
		}
		return map[string]any{"annotation": annotation, "history": history}, nil
	case "dictionary.list":
		terms, err := b.store.ListDictionaryTerms(
			ctx, execution.OwnerUserID, execution.ProjectID,
		)
		if err != nil {
			return nil, err
		}
		return map[string]any{"terms": terms}, nil
	case "dictionary.find":
		terms, err := b.store.ListDictionaryTerms(
			ctx, execution.OwnerUserID, execution.ProjectID,
		)
		if err != nil {
			return nil, err
		}
		query := strings.ToLower(stringArg(arguments, "query"))
		var matches []domain.DictionaryTerm
		for _, term := range terms {
			if strings.Contains(strings.ToLower(term.Canonical), query) {
				matches = append(matches, term)
				continue
			}
			for _, variant := range term.Variants {
				if strings.Contains(strings.ToLower(variant), query) {
					matches = append(matches, term)
					break
				}
			}
		}
		return map[string]any{"terms": matches}, nil
	case "mentions.find":
		results, err := b.searchProjectContext(ctx, execution, stringArg(arguments, "query"))
		if err != nil {
			return nil, err
		}
		return map[string]any{"mentions": results}, nil
	case "assistant_guide.get":
		turns, err := b.store.ListAssistantTurns(
			ctx, execution.OwnerUserID, execution.RunID,
		)
		if err != nil {
			return nil, err
		}
		var guideRevision *int
		for _, turn := range turns {
			if turn.ID == execution.TurnID {
				guideRevision = turn.GuideRevision
				break
			}
		}
		if guideRevision == nil {
			return map[string]any{"exists": false, "pinnedToTurn": true}, nil
		}
		revision, err := b.store.GetAssistantGuideRevision(
			ctx, execution.OwnerUserID, execution.ProjectID, *guideRevision,
		)
		if err != nil {
			return nil, err
		}
		guide := assistant.Guide{
			ProjectID: execution.ProjectID,
			Revision:  revision.Revision,
			Content:   revision.Content,
			Source:    revision.Source,
			CreatedAt: revision.CreatedAt,
			UpdatedAt: revision.CreatedAt,
		}
		return map[string]any{"exists": true, "guide": guide, "pinnedToTurn": true}, nil
	case "project_memory.list":
		records, err := b.store.ListProjectMemory(
			ctx,
			execution.OwnerUserID,
			execution.ProjectID,
			boolArg(arguments, "includeInactive"),
		)
		if err != nil {
			return nil, err
		}
		return map[string]any{"records": records}, nil
	case "project_memory.get":
		record, err := b.store.GetProjectMemory(
			ctx, execution.OwnerUserID, stringArg(arguments, "memoryId"),
		)
		if err != nil || record.ProjectID != execution.ProjectID {
			if err == nil {
				err = pgx.ErrNoRows
			}
			return nil, err
		}
		return toMap("record", record)
	case "project_memory.search":
		records, err := b.store.ListProjectMemory(
			ctx, execution.OwnerUserID, execution.ProjectID, false,
		)
		if err != nil {
			return nil, err
		}
		query := strings.ToLower(stringArg(arguments, "query"))
		var matches []assistant.MemoryRecord
		for _, record := range records {
			if strings.Contains(strings.ToLower(record.Statement), query) ||
				sliceContainsFold(record.Tags, query) {
				matches = append(matches, record)
			}
		}
		return map[string]any{"records": matches}, nil
	case "version.list":
		node, err := b.ownedNode(ctx, execution, stringArg(arguments, "nodeId"))
		if err != nil {
			return nil, err
		}
		versions, err := b.store.ListNamedVersions(ctx, execution.OwnerUserID, node.ID)
		if err != nil {
			return nil, err
		}
		revisions, err := b.store.ListRevisions(ctx, execution.OwnerUserID, node.ID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"versions": versions, "revisions": revisions}, nil
	case "version.compare":
		node, err := b.ownedNode(ctx, execution, stringArg(arguments, "nodeId"))
		if err != nil {
			return nil, err
		}
		revisions, err := b.store.ListRevisions(ctx, execution.OwnerUserID, node.ID)
		if err != nil {
			return nil, err
		}
		left := intArg(arguments, "leftRevision")
		right := intArg(arguments, "rightRevision")
		var leftText, rightText string
		var leftFound, rightFound bool
		for _, revision := range revisions {
			if revision.Revision == left {
				leftText = revision.PlainText
				leftFound = true
			}
			if revision.Revision == right {
				rightText = revision.PlainText
				rightFound = true
			}
		}
		if !leftFound || !rightFound {
			return nil, pgx.ErrNoRows
		}
		return map[string]any{
			"nodeId": node.ID, "leftRevision": left, "rightRevision": right,
			"leftText": leftText, "rightText": rightText,
		}, nil
	case "dictation.list_sessions":
		return b.listDictations(ctx, execution, boundedInt(arguments["limit"], 20, 1, 100))
	case "transcript.get":
		return b.getTranscript(ctx, execution, stringArg(arguments, "sessionId"))
	case "transcript.get_uncertain":
		return b.getUncertainTranscript(
			ctx,
			execution,
			stringArg(arguments, "sessionId"),
			numberArg(arguments, "maximumConfidence"),
		)
	case "audio.get_source_range":
		session, err := b.ownedDictation(ctx, execution, stringArg(arguments, "sessionId"))
		if err != nil {
			return nil, err
		}
		start := int64(intArg(arguments, "startMs"))
		end := int64(intArg(arguments, "endMs"))
		if start < 0 || end <= start || end > session.DurationMS {
			return nil, errors.New("audio range is out of bounds")
		}
		asset, err := b.store.GetAudioAsset(ctx, execution.OwnerUserID, session.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return map[string]any{
				"sessionId": session.ID, "startMs": start, "endMs": end,
				"available": false, "reason": "audio_asset_not_ready",
			}, nil
		}
		if err != nil {
			return nil, err
		}
		if end > asset.DurationMS {
			return nil, errors.New("audio range exceeds the durable audio asset")
		}
		return map[string]any{
			"sessionId": session.ID, "startMs": start, "endMs": end,
			"available": true, "assetId": asset.ID,
			"durationMs": asset.DurationMS, "sha256": asset.SHA256,
		}, nil
	case "export.preview":
		return b.exportPreview(ctx, execution, arguments)
	case "recovery.get_status":
		items, err := b.store.ListRecoveryItems(ctx, execution.OwnerUserID)
		if err != nil {
			return nil, err
		}
		filtered := items[:0]
		for _, item := range items {
			if item.ProjectID == execution.ProjectID {
				filtered = append(filtered, item)
			}
		}
		return map[string]any{"items": filtered}, nil
	case "settings.get_writer_preferences":
		run, err := b.store.GetAssistantRun(ctx, execution.OwnerUserID, execution.RunID)
		if err != nil {
			return nil, err
		}
		preferences := map[string]any{}
		capturedAt := "unavailable_for_legacy_run"
		if snapshotValue, exists := run.ReadContext["writerPreferences"]; exists {
			encoded, encodeErr := json.Marshal(snapshotValue)
			if encodeErr != nil {
				return nil, encodeErr
			}
			var snapshot assistant.WriterPreferencesSnapshot
			if decodeErr := json.Unmarshal(encoded, &snapshot); decodeErr != nil {
				return nil, decodeErr
			}
			snapshotMap, mapErr := toMap("writer", snapshot.Normalized())
			if mapErr != nil {
				return nil, mapErr
			}
			for key, value := range snapshotMap["writer"].(map[string]any) {
				preferences[key] = value
			}
			capturedAt = "assistant_run_start"
		}
		return map[string]any{
			"preferences": preferences,
			"capturedAt":  capturedAt,
		}, nil
	case "assistant.get_capabilities":
		structure, revision, err := b.store.GetProjectStructure(
			ctx, execution.OwnerUserID, execution.ProjectID,
		)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"toolCount":            len(projectmcp.PublicCatalog()),
			"changeReviewRequired": true, "groupedUndo": true,
			"partialApply": true, "structureRevision": revision,
			"structureTemplate": structure.Template,
		}, nil
	default:
		return nil, fmt.Errorf("read tool %q is not implemented", name)
	}
}

func (b *ProjectBackend) searchProjectContext(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	query string,
) ([]map[string]any, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []map[string]any{}, nil
	}
	documents, err := b.store.SearchProject(
		ctx, execution.OwnerUserID, execution.ProjectID, query, "all",
	)
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, 0, len(documents))
	for _, result := range documents {
		results = append(results, map[string]any{
			"nodeId": result.NodeID, "nodeKind": result.NodeKind,
			"breadcrumb": result.Breadcrumb, "snippet": result.Snippet,
			"matchCount": result.MatchCount, "revision": result.Revision,
			"sourceScope": result.SourceScope,
		})
	}
	pattern := "%" + query + "%"
	rows, err := b.store.Pool.Query(ctx, `
		SELECT session.id::text, session.destination_node_id::text,
			COALESCE(node.title, 'Голосовая сессия'),
			COALESCE(draft.plain_text, artifact.text, live.text, '') AS transcript_text,
			COALESCE(draft.revision, 0)
		FROM dictation_sessions session
		LEFT JOIN document_nodes node ON node.id=session.destination_node_id
		LEFT JOIN transcript_drafts draft ON draft.session_id=session.id
		LEFT JOIN LATERAL (
			SELECT value.text
			FROM transcript_artifacts value
			WHERE value.session_id=session.id
			ORDER BY CASE value.kind
				WHEN 'batch_source' THEN 1 WHEN 'live_final' THEN 2 ELSE 3 END,
				value.created_at DESC
			LIMIT 1
		) artifact ON true
		LEFT JOIN LATERAL (
			SELECT string_agg(segment.text, ' ' ORDER BY segment.start_ms) AS text
			FROM live_transcript_segments segment
			WHERE segment.session_id=session.id
		) live ON true
		WHERE session.project_id=$1 AND session.owner_user_id=$2
			AND session.status <> 'trashed'
			AND COALESCE(draft.plain_text, artifact.text, live.text, '') ILIKE $3
		ORDER BY session.updated_at DESC
		LIMIT 100
	`, execution.ProjectID, execution.OwnerUserID, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lowerQuery := strings.ToLower(query)
	for rows.Next() {
		var sessionID, breadcrumb, text string
		var destinationNodeID *string
		var revision int
		if err := rows.Scan(
			&sessionID,
			&destinationNodeID,
			&breadcrumb,
			&text,
			&revision,
		); err != nil {
			return nil, err
		}
		results = append(results, map[string]any{
			"sessionId": sessionID, "destinationNodeId": destinationNodeID,
			"nodeKind": "transcript", "breadcrumb": breadcrumb,
			"snippet":    searchContextSnippet(text, lowerQuery),
			"matchCount": strings.Count(strings.ToLower(text), lowerQuery),
			"revision":   revision, "sourceScope": "transcript",
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func searchContextSnippet(text string, lowerQuery string) string {
	textRunes := []rune(text)
	lowerRunes := []rune(strings.ToLower(text))
	queryRunes := []rune(lowerQuery)
	for index := 0; index+len(queryRunes) <= len(lowerRunes); index++ {
		if string(lowerRunes[index:index+len(queryRunes)]) != lowerQuery {
			continue
		}
		start := max(0, index-60)
		end := min(len(textRunes), index+len(queryRunes)+100)
		return strings.TrimSpace(string(textRunes[start:end]))
	}
	return strings.TrimSpace(text)
}

func (b *ProjectBackend) projectActivity(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	limit int,
) (map[string]any, error) {
	rows, err := b.store.Pool.Query(ctx, `
		SELECT kind, entity_id, payload, occurred_at
		FROM (
			SELECT 'document_revision'::text AS kind, revision.id::text AS entity_id,
				jsonb_build_object(
					'nodeId', node.id::text, 'title', node.title,
					'revision', revision.revision, 'source', revision.source,
					'operationId', revision.operation_id::text
				) AS payload,
				revision.created_at AS occurred_at
			FROM document_revisions revision
			JOIN document_nodes node ON node.id=revision.node_id
			JOIN projects project ON project.id=node.project_id
			WHERE project.id=$1 AND project.owner_user_id=$2
			UNION ALL
			SELECT 'named_version'::text, version.id::text,
				jsonb_build_object(
					'nodeId', node.id::text, 'title', node.title,
					'versionName', version.name, 'revision', version.revision
				),
				version.created_at
			FROM named_versions version
			JOIN document_nodes node ON node.id=version.node_id
			JOIN projects project ON project.id=node.project_id
			WHERE project.id=$1 AND project.owner_user_id=$2
			UNION ALL
			SELECT 'assistant_change_set'::text, change_set.id::text,
				jsonb_build_object(
					'runId', run.id::text, 'state', change_set.state,
					'title', change_set.title, 'riskLevel', change_set.risk_level
				),
				change_set.updated_at
			FROM assistant_change_sets change_set
			JOIN assistant_runs run ON run.id=change_set.run_id
			WHERE run.project_id=$1 AND run.owner_user_id=$2
			UNION ALL
			SELECT 'assistant_event'::text, event.id::text,
				jsonb_build_object(
					'runId', run.id::text, 'turnId', event.turn_id::text,
					'eventType', event.event_type
				),
				event.created_at
			FROM assistant_events event
			JOIN assistant_runs run ON run.id=event.run_id
			WHERE run.project_id=$1 AND run.owner_user_id=$2
		) activity
		ORDER BY occurred_at DESC
		LIMIT $3
	`, execution.ProjectID, execution.OwnerUserID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]map[string]any, 0, limit)
	for rows.Next() {
		var kind, entityID string
		var encoded json.RawMessage
		var occurredAt time.Time
		if err := rows.Scan(&kind, &entityID, &encoded, &occurredAt); err != nil {
			return nil, err
		}
		var payload map[string]any
		if err := json.Unmarshal(encoded, &payload); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"kind": kind, "entityId": entityID,
			"payload": payload, "occurredAt": occurredAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"items": items}, nil
}

func (b *ProjectBackend) buildMutation(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	name string,
	arguments map[string]any,
) (assistant.ChangeOperation, error) {
	policy := assistant.ClassifyTool(name)
	operation := assistant.ChangeOperation{
		ID: uuid.NewString(), GroupKey: groupKeyForTool(name), Selected: true,
		ToolName: name, Arguments: mustJSON(arguments), RiskLevel: policy.RiskLevel,
	}
	switch name {
	case "document.insert", "document.replace_range", "document.append", "document.append_chunk",
		"document.apply_marks", "note.update":
		node, err := b.ownedNode(ctx, execution, stringArg(arguments, "nodeId"))
		if err != nil {
			return operation, err
		}
		if name == "note.update" && node.Kind != preferredNodeType(
			ctx, b.store, execution, "note", true,
		) {
			return operation, errors.New("note.update requires a note-compatible node")
		}
		baseRevision := intArg(arguments, "baseRevision")
		if baseRevision != node.CurrentRevision {
			return operation, assistant.ErrStaleChangeSet
		}
		proposalName := name
		if name == "document.append_chunk" {
			proposalName = "document.append"
		}
		proposed, err := buildDocumentProposal(proposalName, node.Content, arguments)
		if err != nil {
			return operation, err
		}
		before, _ := json.Marshal(node)
		operation.TargetKind = "document"
		operation.TargetID = &node.ID
		operation.BaseRevision = &baseRevision
		operation.BeforeSnapshot = before
		operation.ProposedSnapshot = proposed
		if name == "document.replace_range" || name == "document.apply_marks" {
			ranges, protectedRanges, rangeErr := explicitMutationRanges(name, arguments)
			if rangeErr != nil {
				return operation, rangeErr
			}
			target, resolveErr := assistant.NewExplicitResolvedTarget(
				node.ID,
				node.CurrentRevision,
				node.Content,
				execution.Instruction,
				ranges,
				protectedRanges,
			)
			if resolveErr != nil {
				return operation, resolveErr
			}
			if err := assistant.ValidateProposalScope(node.Content, proposed, target); err != nil {
				return operation, err
			}
			operation.Trace = mustJSON(target)
		}
	case "document.prepare_transfer", "draft.prepare_transfer":
		sourceArgument := "sourceNodeId"
		if name == "draft.prepare_transfer" {
			sourceArgument = "draftNodeId"
		}
		source, err := b.ownedNode(ctx, execution, stringArg(arguments, sourceArgument))
		if err != nil {
			return operation, err
		}
		if name == "draft.prepare_transfer" && source.Kind != preferredNodeType(
			ctx, b.store, execution, "draft", true,
		) {
			return operation, errors.New("draft.prepare_transfer requires a draft-compatible node")
		}
		destination, err := b.ownedNode(
			ctx,
			execution,
			stringArg(arguments, "destinationNodeId"),
		)
		if err != nil {
			return operation, err
		}
		if source.ID == destination.ID {
			return operation, errors.New("transfer source and destination must differ")
		}
		baseRevision := intArg(arguments, "baseRevision")
		if baseRevision != destination.CurrentRevision {
			return operation, assistant.ErrStaleChangeSet
		}
		transferContent, err := cloneDocumentForTransfer(
			source.Content,
			fmt.Sprintf("document:%s@%d", source.ID, source.CurrentRevision),
		)
		if err != nil {
			return operation, err
		}
		proposed, err := buildDocumentProposal(
			"document.append",
			destination.Content,
			map[string]any{"content": json.RawMessage(transferContent)},
		)
		if err != nil {
			return operation, err
		}
		operation.TargetKind = "document"
		operation.TargetID = &destination.ID
		operation.BaseRevision = &baseRevision
		operation.BeforeSnapshot = mustJSON(destination)
		operation.ProposedSnapshot = proposed
		operation.Trace = mustJSON(map[string]any{
			"sourceKind": "document", "sourceNodeId": source.ID,
			"sourceRevision": source.CurrentRevision, "sourceTitle": source.Title,
			"destinationNodeId": destination.ID, "destinationTitle": destination.Title,
		})
	case "transcript.prepare_transfer":
		session, err := b.ownedDictation(ctx, execution, stringArg(arguments, "sessionId"))
		if err != nil {
			return operation, err
		}
		destination, err := b.ownedNode(ctx, execution, stringArg(arguments, "nodeId"))
		if err != nil {
			return operation, err
		}
		baseRevision := intArg(arguments, "baseRevision")
		if baseRevision != destination.CurrentRevision {
			return operation, assistant.ErrStaleChangeSet
		}
		transferContent, sourceLabel, err := transcriptTransferContent(session)
		if err != nil {
			return operation, err
		}
		proposed, err := buildDocumentProposal(
			"document.append",
			destination.Content,
			map[string]any{"content": json.RawMessage(transferContent)},
		)
		if err != nil {
			return operation, err
		}
		operation.TargetKind = "document"
		operation.TargetID = &destination.ID
		operation.BaseRevision = &baseRevision
		operation.BeforeSnapshot = mustJSON(destination)
		operation.ProposedSnapshot = proposed
		operation.Trace = mustJSON(map[string]any{
			"sourceKind": "transcript", "sessionId": session.ID,
			"source": sourceLabel, "destinationNodeId": destination.ID,
			"destinationTitle": destination.Title,
		})
	case "document.set_title", "tree.rename_node", "tree.set_status", "tree.move_node":
		node, err := b.ownedNode(ctx, execution, stringArg(arguments, "nodeId"))
		if err != nil {
			return operation, err
		}
		proposal := nodeProposalFromNode(node)
		switch name {
		case "document.set_title", "tree.rename_node":
			proposal.Title = stringArg(arguments, "title")
		case "tree.set_status":
			proposal.Status = stringArg(arguments, "status")
		case "tree.move_node":
			proposal.ParentID = nullableStringArg(arguments, "parentId")
			position, err := b.positionAfter(
				ctx,
				execution,
				proposal.ParentID,
				nullableStringArg(arguments, "positionAfterId"),
			)
			if err != nil {
				return operation, err
			}
			proposal.Position = position
			if err := b.store.ValidateNodePlacement(
				ctx,
				execution.OwnerUserID,
				execution.ProjectID,
				node.Kind,
				proposal.ParentID,
			); err != nil {
				return operation, err
			}
		}
		before, _ := json.Marshal(node)
		operation.TargetKind = "node"
		operation.TargetID = &node.ID
		operation.BeforeSnapshot = before
		operation.ProposedSnapshot = mustJSON(proposal)
	case "tree.create_node", "document.create_draft", "note.create":
		kind := stringArg(arguments, "kind")
		if name == "document.create_draft" {
			kind = preferredNodeType(ctx, b.store, execution, "draft", true)
		}
		if name == "note.create" {
			kind = preferredNodeType(ctx, b.store, execution, "note", true)
		}
		parentID := nullableStringArg(arguments, "parentId")
		if err := b.store.ValidateNodePlacement(
			ctx, execution.OwnerUserID, execution.ProjectID, kind, parentID,
		); err != nil {
			return operation, err
		}
		proposal := nodeProposal{
			ID: uuid.NewString(), ParentID: parentID, Kind: kind,
			Title: stringArg(arguments, "title"), Status: "draft",
		}
		position, err := b.positionAfter(
			ctx,
			execution,
			parentID,
			nullableStringArg(arguments, "positionAfterId"),
		)
		if err != nil {
			return operation, err
		}
		proposal.Position = position
		if content, exists := arguments["content"]; exists {
			proposal.Content, err = normalizeAssistantContent(content)
			if err != nil {
				return operation, err
			}
		} else {
			proposal.Content = emptyDocument()
		}
		operation.TargetKind = "node"
		operation.TargetID = &proposal.ID
		operation.ProposedSnapshot = mustJSON(proposal)
	case "tree.duplicate_node":
		node, err := b.ownedNode(ctx, execution, stringArg(arguments, "nodeId"))
		if err != nil {
			return operation, err
		}
		proposal := nodeProposalFromNode(node)
		proposal.ID = uuid.NewString()
		proposal.Title += " — копия"
		proposal.Position += 1
		operation.TargetKind = "node"
		operation.TargetID = &proposal.ID
		operation.ProposedSnapshot = mustJSON(proposal)
	case "dictionary.add":
		proposal := dictionaryProposalFromArguments(arguments)
		proposal.ID = uuid.NewString()
		proposal.Revision = 1
		operation.TargetKind = "dictionary"
		operation.TargetID = &proposal.ID
		operation.ProposedSnapshot = mustJSON(proposal)
	case "dictionary.update", "dictionary.set_active":
		term, err := b.dictionaryTerm(
			ctx, execution, stringArg(arguments, "termId"),
		)
		if err != nil {
			return operation, err
		}
		baseRevision := intArg(arguments, "baseRevision")
		if baseRevision != term.Revision {
			return operation, assistant.ErrStaleChangeSet
		}
		proposal := dictionaryProposalFromTerm(term)
		if name == "dictionary.set_active" {
			proposal.Active = boolArg(arguments, "active")
		} else {
			proposal = dictionaryProposalFromArguments(arguments)
			proposal.ID = term.ID
			proposal.Revision = term.Revision + 1
		}
		before, _ := json.Marshal(term)
		operation.TargetKind = "dictionary"
		operation.TargetID = &term.ID
		operation.BaseRevision = &baseRevision
		operation.BeforeSnapshot = before
		operation.ProposedSnapshot = mustJSON(proposal)
	case "assistant_guide.propose_create", "assistant_guide.propose_update":
		var before assistant.Guide
		before, err := b.store.GetAssistantGuide(
			ctx, execution.OwnerUserID, execution.ProjectID, false,
		)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return operation, err
		}
		baseRevision := 0
		if err == nil {
			baseRevision = before.Revision
			operation.BeforeSnapshot = mustJSON(before)
		}
		if name == "assistant_guide.propose_update" &&
			intArg(arguments, "baseRevision") != baseRevision {
			return operation, assistant.ErrStaleChangeSet
		}
		content := stringArg(arguments, "content")
		if err := assistant.ValidateGuide(content); err != nil {
			return operation, err
		}
		operation.TargetKind = "assistant_guide"
		operation.BaseRevision = &baseRevision
		operation.ProposedSnapshot = mustJSON(map[string]any{
			"content": content, "baseRevision": baseRevision,
		})
	case "project_memory.add":
		record := memoryFromArguments(arguments)
		record.ID = uuid.NewString()
		record.ProjectID = execution.ProjectID
		record.Source = "assistant"
		record.Revision = 1
		if err := record.Validate(); err != nil {
			return operation, err
		}
		operation.TargetKind = "project_memory"
		operation.TargetID = &record.ID
		operation.ProposedSnapshot = mustJSON(record)
	case "project_memory.update", "project_memory.set_active",
		"project_memory.set_always_relevant":
		record, err := b.store.GetProjectMemory(
			ctx, execution.OwnerUserID, stringArg(arguments, "memoryId"),
		)
		if err != nil || record.ProjectID != execution.ProjectID {
			if err == nil {
				err = pgx.ErrNoRows
			}
			return operation, err
		}
		baseRevision := intArg(arguments, "baseRevision")
		if record.Revision != baseRevision {
			return operation, assistant.ErrStaleChangeSet
		}
		proposal := record
		switch name {
		case "project_memory.update":
			proposal = memoryFromArguments(arguments)
			proposal.ID = record.ID
			proposal.ProjectID = execution.ProjectID
		case "project_memory.set_active":
			proposal.Active = boolArg(arguments, "active")
		case "project_memory.set_always_relevant":
			proposal.AlwaysRelevant = boolArg(arguments, "alwaysRelevant")
		}
		proposal.Source = "assistant"
		proposal.Revision = record.Revision + 1
		operation.TargetKind = "project_memory"
		operation.TargetID = &record.ID
		operation.BaseRevision = &baseRevision
		operation.BeforeSnapshot = mustJSON(record)
		operation.ProposedSnapshot = mustJSON(proposal)
	case "version.create_snapshot":
		node, err := b.ownedNode(ctx, execution, stringArg(arguments, "nodeId"))
		if err != nil {
			return operation, err
		}
		proposal := map[string]any{
			"id": uuid.NewString(), "nodeId": node.ID,
			"name": stringArg(arguments, "name"), "note": stringArg(arguments, "note"),
		}
		operation.TargetKind = "version"
		operation.TargetID = ptrString(proposal["id"].(string))
		operation.ProposedSnapshot = mustJSON(proposal)
	default:
		return operation, fmt.Errorf("mutation tool %q is not implemented", name)
	}
	if len(operation.ProposedSnapshot) == 0 {
		return operation, errors.New("mutation did not produce a proposal")
	}
	return operation, nil
}

func (b *ProjectBackend) ownedNode(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	nodeID string,
) (domain.DocumentNode, error) {
	node, err := b.store.GetDocumentNode(ctx, execution.OwnerUserID, nodeID)
	if err != nil {
		return domain.DocumentNode{}, err
	}
	if node.ProjectID != execution.ProjectID || node.TrashedAt != nil {
		return domain.DocumentNode{}, pgx.ErrNoRows
	}
	return node, nil
}

func (b *ProjectBackend) documentAtRevision(
	ctx context.Context,
	ownerID string,
	node domain.DocumentNode,
	revision int,
) (domain.DocumentNode, error) {
	if revision == node.CurrentRevision {
		return node, nil
	}
	revisions, err := b.store.ListRevisions(ctx, ownerID, node.ID)
	if err != nil {
		return domain.DocumentNode{}, err
	}
	for _, item := range revisions {
		if item.Revision != revision {
			continue
		}
		node.CurrentRevision = item.Revision
		node.Content = item.Content
		node.PlainText = item.PlainText
		return node, nil
	}
	return domain.DocumentNode{}, pgx.ErrNoRows
}

func (b *ProjectBackend) dictionaryTerm(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	termID string,
) (domain.DictionaryTerm, error) {
	terms, err := b.store.ListDictionaryTerms(
		ctx, execution.OwnerUserID, execution.ProjectID,
	)
	if err != nil {
		return domain.DictionaryTerm{}, err
	}
	for _, term := range terms {
		if term.ID == termID {
			return term, nil
		}
	}
	return domain.DictionaryTerm{}, pgx.ErrNoRows
}

func (b *ProjectBackend) ownedDictation(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	sessionID string,
) (domain.DictationSession, error) {
	session, err := b.store.GetDictationSession(ctx, execution.OwnerUserID, sessionID)
	if err != nil {
		return domain.DictationSession{}, err
	}
	if session.ProjectID != execution.ProjectID {
		return domain.DictationSession{}, pgx.ErrNoRows
	}
	return session, nil
}

func (b *ProjectBackend) listDictations(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	limit int,
) (map[string]any, error) {
	rows, err := b.store.Pool.Query(ctx, `
		SELECT id::text
		FROM dictation_sessions
		WHERE owner_user_id=$1 AND project_id=$2
		ORDER BY created_at DESC
		LIMIT $3
	`, execution.OwnerUserID, execution.ProjectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []domain.DictationSession
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		session, err := b.store.GetDictationSession(ctx, execution.OwnerUserID, id)
		if err != nil {
			return nil, err
		}
		session.Manifest = nil
		sessions = append(sessions, session)
	}
	return map[string]any{"sessions": sessions}, rows.Err()
}

func (b *ProjectBackend) getTranscript(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	sessionID string,
) (map[string]any, error) {
	session, err := b.ownedDictation(ctx, execution, sessionID)
	if err != nil {
		return nil, err
	}
	artifacts, err := b.store.ListTranscriptArtifacts(ctx, execution.OwnerUserID, sessionID)
	if err != nil {
		return nil, err
	}
	draft, err := b.store.GetTranscriptDraft(ctx, execution.OwnerUserID, sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return map[string]any{"session": session, "artifacts": artifacts}, nil
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"session": session, "artifacts": artifacts, "draft": draft}, nil
}

func (b *ProjectBackend) getUncertainTranscript(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	sessionID string,
	maximum float64,
) (map[string]any, error) {
	if _, err := b.ownedDictation(ctx, execution, sessionID); err != nil {
		return nil, err
	}
	artifacts, err := b.store.ListTranscriptArtifacts(ctx, execution.OwnerUserID, sessionID)
	if err != nil {
		return nil, err
	}
	var segments []domain.TranscriptSegment
	for _, artifact := range artifacts {
		for _, segment := range artifact.Segments {
			if segment.Confidence != nil && *segment.Confidence <= maximum {
				segments = append(segments, segment)
			}
		}
	}
	return map[string]any{"segments": segments, "maximumConfidence": maximum}, nil
}

func (b *ProjectBackend) exportPreview(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	arguments map[string]any,
) (map[string]any, error) {
	nodeIDs := stringSliceArg(arguments, "nodeIds")
	var documents []map[string]any
	totalWords := 0
	for _, nodeID := range nodeIDs {
		node, err := b.ownedNode(ctx, execution, nodeID)
		if err != nil {
			return nil, err
		}
		words := len(strings.Fields(node.PlainText))
		totalWords += words
		documents = append(documents, map[string]any{
			"nodeId": node.ID, "title": node.Title, "kind": node.Kind,
			"revision": node.CurrentRevision, "wordCount": words,
		})
	}
	return map[string]any{
		"format": stringArg(arguments, "format"), "documents": documents,
		"wordCount": totalWords, "chapterTitles": boolArg(arguments, "chapterTitles"),
	}, nil
}

func (b *ProjectBackend) positionAfter(
	ctx context.Context,
	execution projectmcp.ExecutionContext,
	parentID *string,
	afterID *string,
) (float64, error) {
	nodes, err := b.store.ListDocumentNodes(
		ctx, execution.OwnerUserID, execution.ProjectID, false,
	)
	if err != nil {
		return 0, err
	}
	position := 0.0
	for _, node := range nodes {
		if sameOptionalString(node.ParentID, parentID) && node.Position > position {
			position = node.Position
		}
		if afterID != nil && node.ID == *afterID &&
			sameOptionalString(node.ParentID, parentID) {
			return node.Position + 1, nil
		}
	}
	if afterID != nil {
		return 0, errors.New("position anchor was not found under the selected parent")
	}
	return position + 1000, nil
}

type nodeProposal struct {
	ID       string          `json:"id"`
	ParentID *string         `json:"parentId"`
	Kind     string          `json:"kind"`
	Title    string          `json:"title"`
	Status   string          `json:"status"`
	Position float64         `json:"position"`
	Content  json.RawMessage `json:"content"`
}

type dictionaryProposal struct {
	ID            string   `json:"id"`
	Canonical     string   `json:"canonical"`
	Variants      []string `json:"variants"`
	Kind          string   `json:"kind"`
	Language      string   `json:"language"`
	Pronunciation string   `json:"pronunciation"`
	Active        bool     `json:"active"`
	Revision      int      `json:"revision"`
}

func nodeProposalFromNode(node domain.DocumentNode) nodeProposal {
	return nodeProposal{
		ID: node.ID, ParentID: node.ParentID, Kind: node.Kind, Title: node.Title,
		Status: node.Status, Position: node.Position, Content: node.Content,
	}
}

func dictionaryProposalFromTerm(term domain.DictionaryTerm) dictionaryProposal {
	return dictionaryProposal{
		ID: term.ID, Canonical: term.Canonical, Variants: term.Variants,
		Kind: term.Kind, Language: term.Language, Pronunciation: term.Pronunciation,
		Active: term.Active, Revision: term.Revision,
	}
}

func dictionaryProposalFromArguments(arguments map[string]any) dictionaryProposal {
	return dictionaryProposal{
		Canonical:     stringArg(arguments, "canonical"),
		Variants:      stringSliceArg(arguments, "variants"),
		Kind:          stringArg(arguments, "kind"),
		Language:      stringArg(arguments, "language"),
		Pronunciation: stringArg(arguments, "pronunciation"),
		Active:        boolArg(arguments, "active"),
	}
}

func memoryFromArguments(arguments map[string]any) assistant.MemoryRecord {
	record := assistant.MemoryRecord{
		Kind:           assistant.MemoryKind(stringArg(arguments, "kind")),
		Statement:      stringArg(arguments, "statement"),
		Scope:          stringArg(arguments, "scope"),
		ScopeNodeID:    nullableStringArg(arguments, "scopeNodeId"),
		Tags:           stringSliceArg(arguments, "tags"),
		Active:         boolArg(arguments, "active"),
		AlwaysRelevant: boolArg(arguments, "alwaysRelevant"),
	}
	if raw, exists := arguments["sourceReferences"]; exists {
		encoded := mustJSON(raw)
		_ = json.Unmarshal(encoded, &record.SourceReferences)
	}
	return record
}

func cloneDocumentForTransfer(
	content json.RawMessage,
	sourceReference string,
) (json.RawMessage, error) {
	var root editor.Node
	if err := json.Unmarshal(content, &root); err != nil {
		return nil, err
	}
	if root.Type != "doc" {
		return nil, errors.New("transfer source is not a structured document")
	}
	var clone func(editor.Node) editor.Node
	clone = func(node editor.Node) editor.Node {
		if node.Attrs != nil {
			attrs := make(map[string]any, len(node.Attrs))
			for key, value := range node.Attrs {
				attrs[key] = value
			}
			node.Attrs = attrs
		}
		if isAssistantIdentifiedBlock(node.Type) {
			if node.Attrs == nil {
				node.Attrs = map[string]any{}
			}
			node.Attrs["blockId"] = uuid.NewString()
		}
		if supportsAssistantSourceRefs(node.Type) {
			if node.Attrs == nil {
				node.Attrs = map[string]any{}
			}
			references := stringValues(node.Attrs["sourceRefs"])
			if sourceReference != "" && !sliceContainsFold(references, sourceReference) {
				references = append(references, sourceReference)
			}
			node.Attrs["sourceRefs"] = references
		}
		for index := range node.Content {
			node.Content[index] = clone(node.Content[index])
		}
		return node
	}
	root = clone(root)
	encoded, err := json.Marshal(root)
	if err != nil {
		return nil, err
	}
	return encoded, editor.Validate(encoded)
}

func transcriptTransferContent(
	session domain.DictationSession,
) (json.RawMessage, string, error) {
	if session.Draft != nil {
		if len(session.Draft.Content) > 0 {
			content, err := cloneDocumentForTransfer(
				session.Draft.Content,
				"transcript:"+session.ID,
			)
			return content, fmt.Sprintf("draft@%d", session.Draft.Revision), err
		}
		if strings.TrimSpace(session.Draft.PlainText) != "" {
			content, err := documentFromPlainText(
				session.Draft.PlainText,
				"transcript:"+session.ID,
			)
			return content, fmt.Sprintf("draft@%d", session.Draft.Revision), err
		}
	}
	artifacts := []*domain.TranscriptArtifact{
		session.SuggestionArtifact,
		session.BatchArtifact,
		session.LiveArtifact,
	}
	for _, artifact := range artifacts {
		if artifact == nil {
			continue
		}
		if len(artifact.Segments) > 0 {
			content, err := editor.FromTranscript(artifact.Segments)
			if err != nil {
				return nil, "", err
			}
			content, err = cloneDocumentForTransfer(content, "transcript:"+session.ID)
			return content, artifact.Kind, err
		}
		if strings.TrimSpace(artifact.Text) != "" {
			content, err := documentFromPlainText(
				artifact.Text,
				"transcript:"+session.ID,
			)
			return content, artifact.Kind, err
		}
	}
	return nil, "", errors.New("dictation session has no transcript content to transfer")
}

func documentFromPlainText(text string, sourceReference string) (json.RawMessage, error) {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	paragraphs := strings.Split(normalized, "\n\n")
	content := make([]editor.Node, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		value := strings.TrimSpace(paragraph)
		if value == "" {
			continue
		}
		references := []string{}
		if sourceReference != "" {
			references = append(references, sourceReference)
		}
		content = append(content, editor.Node{
			Type: "paragraph",
			Attrs: map[string]any{
				"blockId": uuid.NewString(), "sourceRefs": references,
			},
			Content: []editor.Node{{Type: "text", Text: value}},
		})
	}
	if len(content) == 0 {
		return nil, errors.New("transfer source text is empty")
	}
	encoded, err := json.Marshal(editor.Node{Type: "doc", Content: content})
	if err != nil {
		return nil, err
	}
	return encoded, editor.Validate(encoded)
}

func buildDocumentProposal(
	name string,
	current json.RawMessage,
	arguments map[string]any,
) (json.RawMessage, error) {
	if name == "document.apply_marks" {
		return applyDocumentMarks(current, arguments)
	}
	if name == "document.replace_range" {
		return replaceDocumentRanges(current, arguments)
	}
	content, err := normalizeAssistantContent(arguments["content"])
	if err != nil {
		return nil, err
	}
	var root editor.Node
	if err := json.Unmarshal(content, &root); err != nil {
		return nil, err
	}
	if root.Type == "doc" {
		if name == "document.append" {
			var existing editor.Node
			if err := json.Unmarshal(current, &existing); err != nil {
				return nil, err
			}
			existing.Content = append(existing.Content, root.Content...)
			result, _ := json.Marshal(existing)
			return editor.NormalizeSchemaVersion(result)
		}
		return editor.NormalizeSchemaVersion(content)
	}
	var existing editor.Node
	if err := json.Unmarshal(current, &existing); err != nil {
		return nil, err
	}
	if root.Attrs == nil {
		root.Attrs = map[string]any{}
	}
	if _, exists := root.Attrs["blockId"]; !exists && isAssistantIdentifiedBlock(root.Type) {
		root.Attrs["blockId"] = uuid.NewString()
		if supportsAssistantSourceRefs(root.Type) {
			root.Attrs["sourceRefs"] = []string{}
		}
	}
	switch name {
	case "document.append":
		existing.Content = append(existing.Content, root)
	case "document.insert":
		position := intArg(arguments, "position")
		if position < 0 || position > len(existing.Content) {
			return nil, errors.New("document insertion position is out of bounds")
		}
		existing.Content = append(
			existing.Content[:position],
			append([]editor.Node{root}, existing.Content[position:]...)...,
		)
	default:
		return nil, errors.New("unsupported document proposal")
	}
	result, _ := json.Marshal(existing)
	return editor.NormalizeSchemaVersion(result)
}

type documentRangeEdit struct {
	BlockID      string `json:"blockId"`
	Start        int    `json:"start"`
	End          int    `json:"end"`
	ExpectedText string `json:"expectedText"`
	Replacement  string `json:"replacement"`
}

func replaceDocumentRanges(
	current json.RawMessage,
	arguments map[string]any,
) (json.RawMessage, error) {
	var root editor.Node
	if err := json.Unmarshal(current, &root); err != nil {
		return nil, err
	}
	var edits []documentRangeEdit
	if err := json.Unmarshal(mustJSON(arguments["edits"]), &edits); err != nil {
		return nil, errors.New("document range edits are invalid")
	}
	if len(edits) == 0 {
		return nil, errors.New("at least one document range edit is required")
	}
	byBlock := map[string][]documentRangeEdit{}
	for _, edit := range edits {
		if strings.TrimSpace(edit.BlockID) == "" || edit.Start < 0 || edit.End < edit.Start {
			return nil, errors.New("document range edit is invalid")
		}
		byBlock[edit.BlockID] = append(byBlock[edit.BlockID], edit)
	}
	blockIDs := editor.EffectiveTopLevelBlockIDs(root.Content)
	foundBlocks := map[string]bool{}
	for index := range root.Content {
		blockID := blockIDs[index]
		blockEdits := byBlock[blockID]
		if len(blockEdits) == 0 {
			continue
		}
		if currentBlockID, _ := root.Content[index].Attrs["blockId"].(string); strings.TrimSpace(currentBlockID) == "" {
			if root.Content[index].Attrs == nil {
				root.Content[index].Attrs = map[string]any{}
			}
			root.Content[index].Attrs["blockId"] = blockID
		}
		foundBlocks[blockID] = true
		blockRunes := []rune(inlineNodeText(root.Content[index].Content))
		for _, edit := range blockEdits {
			if edit.End > len(blockRunes) ||
				string(blockRunes[edit.Start:edit.End]) != edit.ExpectedText {
				return nil, errors.New(
					"document range expected text does not match the current revision",
				)
			}
		}
		sort.Slice(blockEdits, func(left int, right int) bool {
			if blockEdits[left].Start == blockEdits[right].Start {
				return blockEdits[left].End > blockEdits[right].End
			}
			return blockEdits[left].Start > blockEdits[right].Start
		})
		previousStart := int(^uint(0) >> 1)
		for _, edit := range blockEdits {
			if edit.End > previousStart {
				return nil, errors.New("document range edits overlap")
			}
			content, changed, err := replaceInlineTextRange(
				root.Content[index].Content,
				edit.Start,
				edit.End,
				edit.Replacement,
			)
			if err != nil {
				return nil, err
			}
			if !changed {
				return nil, errors.New("document range edit did not change a range")
			}
			root.Content[index].Content = content
			previousStart = edit.Start
		}
	}
	if len(foundBlocks) != len(byBlock) {
		return nil, errors.New("document range edit block was not found")
	}
	result, err := json.Marshal(root)
	if err != nil {
		return nil, err
	}
	return editor.NormalizeSchemaVersion(result)
}

func inlineNodeText(nodes []editor.Node) string {
	var builder strings.Builder
	var appendNodes func([]editor.Node)
	appendNodes = func(current []editor.Node) {
		for _, node := range current {
			builder.WriteString(node.Text)
			appendNodes(node.Content)
		}
	}
	appendNodes(nodes)
	return builder.String()
}

func findExactDocumentAnchors(
	node domain.DocumentNode,
	text string,
	blockID *string,
) (map[string]any, error) {
	needle := []rune(text)
	if len(needle) == 0 {
		return nil, errors.New("exact document text must not be empty")
	}
	var root editor.Node
	if err := json.Unmarshal(node.Content, &root); err != nil {
		return nil, err
	}
	const maximumMatches = 100
	matches := make([]map[string]any, 0)
	blockIDs := editor.EffectiveTopLevelBlockIDs(root.Content)
	for index, block := range root.Content {
		currentBlockID := blockIDs[index]
		if blockID != nil && currentBlockID != *blockID {
			continue
		}
		haystack := []rune(inlineNodeText(block.Content))
		for start := 0; start+len(needle) <= len(haystack); start++ {
			equal := true
			for offset := range needle {
				if haystack[start+offset] != needle[offset] {
					equal = false
					break
				}
			}
			if !equal {
				continue
			}
			matches = append(matches, map[string]any{
				"blockId":      currentBlockID,
				"start":        start,
				"end":          start + len(needle),
				"expectedText": text,
			})
			if len(matches) == maximumMatches {
				return map[string]any{
					"nodeId": node.ID, "revision": node.CurrentRevision,
					"matches": matches, "matchCount": len(matches), "truncated": true,
					"coordinateSystem": "block-local Unicode code points",
				}, nil
			}
		}
	}
	return map[string]any{
		"nodeId": node.ID, "revision": node.CurrentRevision,
		"matches": matches, "matchCount": len(matches), "truncated": false,
		"coordinateSystem": "block-local Unicode code points",
	}, nil
}

func findExactDocumentAnchorsBatch(
	node domain.DocumentNode,
	value any,
) (map[string]any, error) {
	var queries []struct {
		Text    string  `json:"text"`
		BlockID *string `json:"blockId"`
	}
	if err := json.Unmarshal(mustJSON(value), &queries); err != nil || len(queries) == 0 {
		return nil, errors.New("exact document query batch must not be empty")
	}
	const maximumBatchQueries = 50
	if len(queries) > maximumBatchQueries {
		return nil, errors.New("exact document query batch is too large")
	}
	results := make([]map[string]any, 0, len(queries))
	for index, query := range queries {
		result, err := findExactDocumentAnchors(node, query.Text, query.BlockID)
		if err != nil {
			return nil, fmt.Errorf("exact document query %d: %w", index, err)
		}
		results = append(results, map[string]any{
			"queryIndex": index,
			"text":       query.Text,
			"blockId":    query.BlockID,
			"matches":    result["matches"],
			"matchCount": result["matchCount"],
			"truncated":  result["truncated"],
		})
	}
	return map[string]any{
		"nodeId": node.ID, "revision": node.CurrentRevision, "results": results,
		"coordinateSystem": "block-local Unicode code points",
	}, nil
}

func replaceInlineTextRange(
	nodes []editor.Node,
	start int,
	end int,
	replacement string,
) ([]editor.Node, bool, error) {
	if start < 0 || end < start || (start == end && replacement == "") {
		return nil, false, errors.New("document range edit is invalid")
	}
	cursor := 0
	inserted := false
	result, changed := replaceInlineTextNodes(
		nodes,
		&cursor,
		start,
		end,
		replacement,
		&inserted,
	)
	if start == end && !inserted && start == cursor {
		result = append(result, editor.Node{Type: "text", Text: replacement})
		inserted = true
		changed = true
	}
	if end > cursor || !inserted {
		return nil, false, errors.New("document range edit is out of bounds")
	}
	return result, changed, nil
}

func replaceInlineTextNodes(
	nodes []editor.Node,
	cursor *int,
	start int,
	end int,
	replacement string,
	inserted *bool,
) ([]editor.Node, bool) {
	result := make([]editor.Node, 0, len(nodes)+1)
	changed := false
	for _, node := range nodes {
		if node.Type != "text" {
			if len(node.Content) > 0 {
				children, childChanged := replaceInlineTextNodes(
					node.Content,
					cursor,
					start,
					end,
					replacement,
					inserted,
				)
				node.Content = children
				changed = changed || childChanged
			}
			result = append(result, node)
			continue
		}
		runes := []rune(node.Text)
		nodeStart := *cursor
		nodeEnd := nodeStart + len(runes)
		*cursor = nodeEnd
		if start == end {
			if *inserted || start < nodeStart || start > nodeEnd {
				result = append(result, node)
				continue
			}
			local := start - nodeStart
			if local > 0 {
				prefix := node
				prefix.Text = string(runes[:local])
				result = append(result, prefix)
			}
			middle := node
			middle.Text = replacement
			result = append(result, middle)
			if local < len(runes) {
				suffix := node
				suffix.Text = string(runes[local:])
				result = append(result, suffix)
			}
			*inserted = true
			changed = true
			continue
		}
		overlapStart := max(start, nodeStart)
		overlapEnd := min(end, nodeEnd)
		if overlapStart >= overlapEnd {
			result = append(result, node)
			continue
		}
		localStart := overlapStart - nodeStart
		localEnd := overlapEnd - nodeStart
		if localStart > 0 {
			prefix := node
			prefix.Text = string(runes[:localStart])
			result = append(result, prefix)
		}
		if !*inserted {
			if replacement != "" {
				middle := node
				middle.Text = replacement
				result = append(result, middle)
			}
			*inserted = true
		}
		if localEnd < len(runes) {
			suffix := node
			suffix.Text = string(runes[localEnd:])
			result = append(result, suffix)
		}
		changed = true
	}
	return result, changed
}

type assistantContentEnvelope struct {
	Type    string                  `json:"type"`
	Blocks  []assistantContentBlock `json:"blocks"`
	Content []editor.Node           `json:"content"`
}

type assistantContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func normalizeAssistantContent(value any) (json.RawMessage, error) {
	if text, ok := value.(string); ok {
		return documentFromPlainText(text, "assistant")
	}
	encoded := mustJSON(value)
	var envelope assistantContentEnvelope
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		return nil, errors.New("assistant content is not valid JSON")
	}
	if len(envelope.Blocks) > 0 {
		nodes := make([]editor.Node, 0, len(envelope.Blocks))
		for _, block := range envelope.Blocks {
			blockType := block.Type
			if blockType == "" {
				blockType = "paragraph"
			}
			if blockType != "paragraph" && blockType != "heading" &&
				blockType != "blockquote" {
				return nil, fmt.Errorf("unsupported assistant block type %q", blockType)
			}
			attrs := map[string]any{
				"blockId": uuid.NewString(), "sourceRefs": []string{"assistant"},
			}
			if blockType == "heading" {
				attrs["level"] = 2
			}
			node := editor.Node{Type: blockType, Attrs: attrs}
			if block.Text != "" {
				node.Content = []editor.Node{{Type: "text", Text: block.Text}}
			}
			nodes = append(nodes, node)
		}
		encoded, _ = json.Marshal(editor.Node{Type: "doc", Content: nodes})
		return editor.NormalizeSchemaVersion(encoded)
	}
	if envelope.Type == "" && len(envelope.Content) == 0 {
		return nil, errors.New("assistant content must contain a document or blocks")
	}
	if envelope.Type != "" && envelope.Type != "doc" {
		var single editor.Node
		if err := json.Unmarshal(encoded, &single); err != nil {
			return nil, err
		}
		ensureAssistantBlockMetadata(&single)
		encoded, _ = json.Marshal(single)
		return encoded, nil
	}
	for index := range envelope.Content {
		ensureAssistantBlockMetadata(&envelope.Content[index])
	}
	encoded, _ = json.Marshal(editor.Node{Type: "doc", Content: envelope.Content})
	return editor.NormalizeSchemaVersion(encoded)
}

func ensureAssistantBlockMetadata(node *editor.Node) {
	if node == nil {
		return
	}
	if node.Attrs == nil {
		node.Attrs = map[string]any{}
	}
	if isAssistantIdentifiedBlock(node.Type) {
		if blockID, _ := node.Attrs["blockId"].(string); strings.TrimSpace(blockID) == "" {
			node.Attrs["blockId"] = uuid.NewString()
		}
	}
	if supportsAssistantSourceRefs(node.Type) {
		if _, exists := node.Attrs["sourceRefs"]; !exists {
			node.Attrs["sourceRefs"] = []string{"assistant"}
		}
	}
	for index := range node.Content {
		ensureAssistantBlockMetadata(&node.Content[index])
	}
}

func isAssistantIdentifiedBlock(nodeType string) bool {
	switch nodeType {
	case "paragraph", "heading", "blockquote", "codeBlock", "callout", "table",
		"imageBlock", "mermaidBlock":
		return true
	default:
		return false
	}
}

func supportsAssistantSourceRefs(nodeType string) bool {
	return nodeType == "paragraph" || nodeType == "heading" || nodeType == "blockquote"
}

func stringValues(value any) []string {
	switch values := value.(type) {
	case []string:
		return append([]string(nil), values...)
	case []any:
		result := make([]string, 0, len(values))
		for _, item := range values {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func applyDocumentMarks(
	current json.RawMessage,
	arguments map[string]any,
) (json.RawMessage, error) {
	var root editor.Node
	if err := json.Unmarshal(current, &root); err != nil {
		return nil, err
	}
	var requestedMarks []struct {
		Type string  `json:"type"`
		Href *string `json:"href"`
	}
	if encoded := mustJSON(arguments["marks"]); json.Unmarshal(encoded, &requestedMarks) != nil {
		return nil, errors.New("document marks are invalid")
	}
	marks := make([]editor.Mark, 0, len(requestedMarks))
	for _, requested := range requestedMarks {
		mark := editor.Mark{Type: requested.Type}
		if requested.Type == "link" && requested.Href != nil {
			mark.Attrs = map[string]any{"href": *requested.Href}
		}
		marks = append(marks, mark)
	}
	blockID := stringArg(arguments, "blockId")
	start := intArg(arguments, "start")
	end := intArg(arguments, "end")
	if blockID == "" || start < 0 || end <= start {
		return nil, errors.New("document mark range is invalid")
	}
	found := false
	blockIDs := editor.EffectiveTopLevelBlockIDs(root.Content)
	for index := range root.Content {
		if blockIDs[index] != blockID {
			continue
		}
		if currentBlockID, _ := root.Content[index].Attrs["blockId"].(string); strings.TrimSpace(currentBlockID) == "" {
			if root.Content[index].Attrs == nil {
				root.Content[index].Attrs = map[string]any{}
			}
			root.Content[index].Attrs["blockId"] = blockID
		}
		found = true
		cursor := 0
		content, changed, err := markInlineNodes(
			root.Content[index].Content,
			&cursor,
			start,
			end,
			marks,
		)
		if err != nil {
			return nil, err
		}
		if !changed || end > cursor {
			return nil, errors.New("document mark range is out of bounds")
		}
		root.Content[index].Content = content
		break
	}
	if !found {
		return nil, errors.New("document mark block was not found")
	}
	result, err := json.Marshal(root)
	if err != nil {
		return nil, err
	}
	return editor.NormalizeSchemaVersion(result)
}

func markInlineNodes(
	nodes []editor.Node,
	cursor *int,
	start int,
	end int,
	marks []editor.Mark,
) ([]editor.Node, bool, error) {
	var result []editor.Node
	changed := false
	for _, node := range nodes {
		if node.Type != "text" {
			if len(node.Content) > 0 {
				children, childChanged, err := markInlineNodes(
					node.Content,
					cursor,
					start,
					end,
					marks,
				)
				if err != nil {
					return nil, false, err
				}
				node.Content = children
				changed = changed || childChanged
			}
			result = append(result, node)
			continue
		}
		runes := []rune(node.Text)
		nodeStart := *cursor
		nodeEnd := nodeStart + len(runes)
		*cursor = nodeEnd
		overlapStart := max(start, nodeStart)
		overlapEnd := min(end, nodeEnd)
		if overlapStart >= overlapEnd {
			result = append(result, node)
			continue
		}
		localStart := overlapStart - nodeStart
		localEnd := overlapEnd - nodeStart
		if localStart > 0 {
			prefix := node
			prefix.Text = string(runes[:localStart])
			result = append(result, prefix)
		}
		middle := node
		middle.Text = string(runes[localStart:localEnd])
		middle.Marks = mergeMarks(middle.Marks, marks)
		result = append(result, middle)
		if localEnd < len(runes) {
			suffix := node
			suffix.Text = string(runes[localEnd:])
			result = append(result, suffix)
		}
		changed = true
	}
	return result, changed, nil
}

func mergeMarks(existing []editor.Mark, additions []editor.Mark) []editor.Mark {
	result := append([]editor.Mark(nil), existing...)
	for _, addition := range additions {
		found := false
		for _, current := range result {
			if current.Type == addition.Type {
				found = true
				break
			}
		}
		if !found {
			result = append(result, addition)
		}
	}
	return result
}

func emptyDocument() json.RawMessage {
	return mustJSON(editor.Node{
		Type:  "doc",
		Attrs: map[string]any{"schemaVersion": 1},
		Content: []editor.Node{{
			Type: "paragraph",
			Attrs: map[string]any{
				"blockId": uuid.NewString(), "sourceRefs": []string{},
			},
		}},
	})
}

func preferredNodeType(
	ctx context.Context,
	dataStore *store.Store,
	execution projectmcp.ExecutionContext,
	preferred string,
	contentBearing bool,
) string {
	structure, _, err := dataStore.GetProjectStructure(
		ctx, execution.OwnerUserID, execution.ProjectID,
	)
	if err != nil {
		return preferred
	}
	for _, nodeType := range structure.NodeTypes {
		if nodeType.Key == preferred {
			return preferred
		}
	}
	for _, nodeType := range structure.NodeTypes {
		if nodeType.ContentBearing == contentBearing && nodeType.CanBeRoot {
			return nodeType.Key
		}
	}
	return preferred
}

func documentReferences(content json.RawMessage) []map[string]any {
	var root editor.Node
	if json.Unmarshal(content, &root) != nil {
		return nil
	}
	var result []map[string]any
	var walk func(editor.Node)
	walk = func(node editor.Node) {
		blockID, _ := node.Attrs["blockId"].(string)
		if blockID != "" {
			if refs, exists := node.Attrs["sourceRefs"]; exists {
				result = append(result, map[string]any{
					"blockId": blockID, "sourceRefs": refs,
				})
			}
		}
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(root)
	return result
}

func groupKeyForTool(name string) string {
	domain := strings.SplitN(name, ".", 2)[0]
	switch domain {
	case "document", "tree", "note", "draft":
		return "document"
	case "dictionary":
		return "dictionary"
	case "project_memory":
		return "memory"
	case "assistant_guide":
		return "guide"
	case "version":
		return "version"
	default:
		return domain
	}
}

func toMap(key string, value any) (map[string]any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil, err
	}
	return map[string]any{key: decoded}, nil
}

func mustJSON(value any) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}

func stringArg(arguments map[string]any, name string) string {
	value, _ := arguments[name].(string)
	return strings.TrimSpace(value)
}

func nullableStringArg(arguments map[string]any, name string) *string {
	value, ok := arguments[name].(string)
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return nil
	}
	return &value
}

func intArg(arguments map[string]any, name string) int {
	switch value := arguments[name].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case json.Number:
		number, _ := strconv.Atoi(string(value))
		return number
	default:
		return 0
	}
}

func boundedInt(value any, fallback int, minimum int, maximum int) int {
	result := intArg(map[string]any{"value": value}, "value")
	if result == 0 {
		result = fallback
	}
	if result < minimum {
		return minimum
	}
	if result > maximum {
		return maximum
	}
	return result
}

func numberArg(arguments map[string]any, name string) float64 {
	value, _ := arguments[name].(float64)
	return value
}

func boolArg(arguments map[string]any, name string) bool {
	value, _ := arguments[name].(bool)
	return value
}

func stringSliceArg(arguments map[string]any, name string) []string {
	values, _ := arguments[name].([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			result = append(result, strings.TrimSpace(text))
		}
	}
	if direct, ok := arguments[name].([]string); ok {
		return direct
	}
	return result
}

func explicitMutationRanges(
	name string,
	arguments map[string]any,
) ([]assistant.TextRange, []assistant.TextRange, error) {
	var ranges []assistant.TextRange
	switch name {
	case "document.replace_range":
		edits, err := objectSliceArg(arguments, "edits")
		if err != nil || len(edits) == 0 {
			return nil, nil, errors.New("explicit document edits are required")
		}
		for index, edit := range edits {
			ranges = append(ranges, assistant.TextRange{
				BlockID: stringArg(edit, "blockId"),
				Start:   intArg(edit, "start"),
				End:     intArg(edit, "end"),
				Reason:  fmt.Sprintf("explicit edit %d", index+1),
			})
		}
	case "document.apply_marks":
		ranges = []assistant.TextRange{{
			BlockID: stringArg(arguments, "blockId"),
			Start:   intArg(arguments, "start"),
			End:     intArg(arguments, "end"),
			Reason:  "explicit formatting range",
		}}
	default:
		return nil, nil, errors.New("tool does not use explicit text ranges")
	}
	protectedArguments, err := objectSliceArg(arguments, "protectedRanges")
	if err != nil {
		return nil, nil, errors.New("protected text ranges are invalid")
	}
	protected := make([]assistant.TextRange, 0, len(protectedArguments))
	for _, item := range protectedArguments {
		protected = append(protected, assistant.TextRange{
			BlockID: stringArg(item, "blockId"),
			Start:   intArg(item, "start"),
			End:     intArg(item, "end"),
			Reason:  stringArg(item, "reason"),
		})
	}
	return ranges, protected, nil
}

func objectSliceArg(arguments map[string]any, name string) ([]map[string]any, error) {
	value, exists := arguments[name]
	if !exists || value == nil {
		return nil, nil
	}
	switch items := value.(type) {
	case []any:
		result := make([]map[string]any, 0, len(items))
		for _, item := range items {
			object, ok := item.(map[string]any)
			if !ok {
				return nil, errors.New("array item is not an object")
			}
			result = append(result, object)
		}
		return result, nil
	case []map[string]any:
		return items, nil
	default:
		return nil, errors.New("value is not an object array")
	}
}

func sliceContainsFold(values []string, query string) bool {
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

func sameOptionalString(left *string, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func ptrString(value string) *string {
	return &value
}

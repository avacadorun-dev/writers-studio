package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"writers.studio/internal/assistant"
	canvasdocument "writers.studio/internal/canvas"
	"writers.studio/internal/documentannotation"
	"writers.studio/internal/domain"
	"writers.studio/internal/editor"
	"writers.studio/internal/markdowntree"
)

type assistantChangeSetTxGuard func(context.Context, pgx.Tx) error

func jsonEqual(left, right json.RawMessage) bool {
	var leftValue, rightValue any
	return json.Unmarshal(left, &leftValue) == nil &&
		json.Unmarshal(right, &rightValue) == nil &&
		reflect.DeepEqual(leftValue, rightValue)
}

func (s *Store) ApplyAssistantChangeSet(
	ctx context.Context,
	ownerID string,
	runID string,
	changeSetID string,
	selectedGroups []string,
	annotationResolutionOverrides ...map[string]bool,
) (assistant.ChangeSet, error) {
	resolutionOverrides := map[string]bool{}
	if len(annotationResolutionOverrides) > 0 && annotationResolutionOverrides[0] != nil {
		resolutionOverrides = annotationResolutionOverrides[0]
	}
	return s.applyAssistantChangeSet(
		ctx, ownerID, runID, changeSetID, selectedGroups, resolutionOverrides, nil,
	)
}

func (s *Store) applyAssistantChangeSet(
	ctx context.Context,
	ownerID string,
	runID string,
	changeSetID string,
	selectedGroups []string,
	resolutionOverrides map[string]bool,
	guard assistantChangeSetTxGuard,
) (assistant.ChangeSet, error) {
	operationID := uuid.NewString()
	var terminalError error
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var projectID string
		var turnID string
		var state assistant.ChangeSetState
		var baseFingerprint string
		if err := tx.QueryRow(ctx, `
			SELECT r.project_id::text, c.turn_id::text, c.state, c.base_fingerprint
			FROM assistant_change_sets c
			JOIN assistant_runs r ON r.id=c.run_id
			WHERE c.id=$1 AND c.run_id=$2 AND r.owner_user_id=$3
			FOR UPDATE OF c, r
		`, changeSetID, runID, ownerID).Scan(
			&projectID,
			&turnID,
			&state,
			&baseFingerprint,
		); err != nil {
			return err
		}
		if state != assistant.ChangeStaged {
			return errors.New("assistant change set is not staged")
		}
		if guard != nil {
			if err := guard(ctx, tx); err != nil {
				return err
			}
		}
		currentFingerprint, err := projectFingerprint(ctx, tx, ownerID, projectID)
		if err != nil {
			return err
		}
		if currentFingerprint != baseFingerprint {
			if _, err := tx.Exec(ctx, `
				UPDATE assistant_change_sets
				SET state='stale', updated_at=now()
				WHERE id=$1
			`, changeSetID); err != nil {
				return err
			}
			if err := finishAssistantReviewTx(
				ctx, tx, runID, turnID, assistant.TurnFailed, "stale_change_set",
			); err != nil {
				return err
			}
			terminalError = assistant.ErrStaleChangeSet
			return nil
		}
		operations, err := loadAssistantOperationsForUpdate(ctx, tx, changeSetID)
		if err != nil {
			return err
		}
		selected, err := selectAssistantGroups(operations, selectedGroups)
		if err != nil {
			return err
		}
		if err := validateAnnotationResolutionOverrides(
			operations, selected, resolutionOverrides,
		); err != nil {
			return err
		}
		for _, operation := range operations {
			shouldApply := selected[operation.GroupKey]
			if _, err := tx.Exec(ctx, `
				UPDATE assistant_change_operations
				SET selected=$2
				WHERE id=$1
			`, operation.ID, shouldApply); err != nil {
				return err
			}
			if !shouldApply {
				continue
			}
			if err := applyAssistantOperation(
				ctx,
				tx,
				ownerID,
				projectID,
				runID,
				changeSetID,
				operationID,
				operation,
				resolutionOverrides,
			); err != nil {
				return fmt.Errorf("apply %s: %w", operation.ToolName, err)
			}
			if _, err := tx.Exec(ctx, `
				UPDATE assistant_change_operations SET applied=true WHERE id=$1
			`, operation.ID); err != nil {
				return err
			}
		}
		now := time.Now()
		if _, err := tx.Exec(ctx, `
			UPDATE assistant_change_sets
			SET state='applied', operation_id=$2, applied_at=$3, updated_at=$3
			WHERE id=$1
		`, changeSetID, operationID, now); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE assistant_runs
			SET status='active', error_code=NULL, ended_at=NULL, updated_at=$2
			WHERE id=$1
		`, runID, now); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE assistant_turns
			SET state='completed', completed_at=COALESCE(completed_at, $2)
			WHERE id=$1 AND state='awaiting_review'
		`, turnID, now); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, assistant.ErrStaleChangeSet) {
			changeSet, staleErr := s.MarkAssistantChangeSetStale(
				ctx, ownerID, runID, changeSetID,
			)
			if staleErr == nil {
				return changeSet, assistant.ErrStaleChangeSet
			}
			if !errors.Is(staleErr, pgx.ErrNoRows) {
				return assistant.ChangeSet{}, staleErr
			}
		}
		return assistant.ChangeSet{}, err
	}
	if terminalError != nil {
		changeSet, getErr := s.GetAssistantChangeSet(ctx, runID, changeSetID)
		if getErr != nil {
			return assistant.ChangeSet{}, getErr
		}
		return changeSet, terminalError
	}
	return s.GetAssistantChangeSet(ctx, runID, changeSetID)
}

func validateAnnotationResolutionOverrides(
	operations []assistant.ChangeOperation,
	selected map[string]bool,
	overrides map[string]bool,
) error {
	if len(overrides) == 0 {
		return nil
	}
	allowed := map[string]bool{}
	for _, operation := range operations {
		if !selected[operation.GroupKey] || operation.ToolName != "annotation.propose_fixes" {
			continue
		}
		_, _, expectations, err := assistantAnnotationEdits(operation)
		if err != nil {
			return err
		}
		for annotationID := range expectations {
			allowed[annotationID] = true
		}
	}
	for annotationID := range overrides {
		if !allowed[annotationID] {
			return errors.New("annotation resolution override is outside selected changes")
		}
	}
	return nil
}

func (s *Store) RejectAssistantChangeSet(
	ctx context.Context,
	ownerID string,
	runID string,
	changeSetID string,
) (assistant.ChangeSet, error) {
	return s.rejectAssistantChangeSet(ctx, ownerID, runID, changeSetID, nil)
}

func (s *Store) rejectAssistantChangeSet(
	ctx context.Context,
	ownerID string,
	runID string,
	changeSetID string,
	guard assistantChangeSetTxGuard,
) (assistant.ChangeSet, error) {
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var turnID string
		if err := tx.QueryRow(ctx, `
			SELECT c.turn_id::text
			FROM assistant_change_sets c
			JOIN assistant_runs r ON r.id=c.run_id
			WHERE c.id=$1 AND c.run_id=$2 AND c.state='staged'
				AND r.owner_user_id=$3
			FOR UPDATE OF c, r
		`, changeSetID, runID, ownerID).Scan(&turnID); err != nil {
			return err
		}
		if guard != nil {
			if err := guard(ctx, tx); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE assistant_change_sets
			SET state='rejected', rejected_at=now(), updated_at=now()
			WHERE id=$1
		`, changeSetID); err != nil {
			return err
		}
		return finishAssistantReviewTx(
			ctx, tx, runID, turnID, assistant.TurnCompleted, "",
		)
	})
	if err != nil {
		return assistant.ChangeSet{}, err
	}
	return s.GetAssistantChangeSet(ctx, runID, changeSetID)
}

func (s *Store) MarkAssistantChangeSetStale(
	ctx context.Context,
	ownerID string,
	runID string,
	changeSetID string,
) (assistant.ChangeSet, error) {
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var turnID string
		if err := tx.QueryRow(ctx, `
			SELECT c.turn_id::text
			FROM assistant_change_sets c
			JOIN assistant_runs r ON r.id=c.run_id
			WHERE c.id=$1 AND c.run_id=$2 AND c.state='staged'
				AND r.owner_user_id=$3
			FOR UPDATE OF c, r
		`, changeSetID, runID, ownerID).Scan(&turnID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE assistant_change_sets
			SET state='stale', updated_at=now()
			WHERE id=$1
		`, changeSetID); err != nil {
			return err
		}
		return finishAssistantReviewTx(
			ctx, tx, runID, turnID, assistant.TurnFailed, "stale_change_set",
		)
	})
	if err != nil {
		return assistant.ChangeSet{}, err
	}
	return s.GetAssistantChangeSet(ctx, runID, changeSetID)
}

func finishAssistantReviewTx(
	ctx context.Context,
	tx pgx.Tx,
	runID string,
	turnID string,
	turnState assistant.TurnState,
	errorCode string,
) error {
	var code any
	if errorCode != "" {
		code = errorCode
	}
	if _, err := tx.Exec(ctx, `
		UPDATE assistant_runs
		SET status='active', error_code=$2, ended_at=NULL, updated_at=now()
		WHERE id=$1
	`, runID, code); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		UPDATE assistant_turns
		SET state=$2, error_code=COALESCE($3, error_code),
			completed_at=COALESCE(completed_at, now())
		WHERE id=$1 AND state='awaiting_review'
	`, turnID, turnState, code)
	return err
}

func (s *Store) UndoAssistantChangeSet(
	ctx context.Context,
	ownerID string,
	runID string,
	changeSetID string,
) (assistant.ChangeSet, error) {
	return s.undoAssistantChangeSet(ctx, ownerID, runID, changeSetID, nil)
}

func (s *Store) undoAssistantChangeSet(
	ctx context.Context,
	ownerID string,
	runID string,
	changeSetID string,
	guard assistantChangeSetTxGuard,
) (assistant.ChangeSet, error) {
	undoOperationID := uuid.NewString()
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var projectID string
		var state assistant.ChangeSetState
		if err := tx.QueryRow(ctx, `
			SELECT r.project_id::text, c.state
			FROM assistant_change_sets c
			JOIN assistant_runs r ON r.id=c.run_id
			WHERE c.id=$1 AND c.run_id=$2 AND r.owner_user_id=$3
			FOR UPDATE OF c, r
		`, changeSetID, runID, ownerID).Scan(&projectID, &state); err != nil {
			return err
		}
		if state != assistant.ChangeApplied {
			return errors.New("assistant change set is not applied")
		}
		if guard != nil {
			if err := guard(ctx, tx); err != nil {
				return err
			}
		}
		operations, err := loadAssistantOperationsForUpdate(ctx, tx, changeSetID)
		if err != nil {
			return err
		}
		slices.Reverse(operations)
		for _, operation := range operations {
			if !operation.Applied {
				continue
			}
			if err := undoAssistantOperation(
				ctx,
				tx,
				ownerID,
				projectID,
				runID,
				changeSetID,
				undoOperationID,
				operation,
			); err != nil {
				return fmt.Errorf("undo %s: %w", operation.ToolName, err)
			}
		}
		_, err = tx.Exec(ctx, `
			UPDATE assistant_change_sets
			SET state='undone', undone_at=now(), updated_at=now()
			WHERE id=$1
		`, changeSetID)
		return err
	})
	if err != nil {
		return assistant.ChangeSet{}, err
	}
	return s.GetAssistantChangeSet(ctx, runID, changeSetID)
}

func loadAssistantOperationsForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	changeSetID string,
) ([]assistant.ChangeOperation, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, position, group_key, selected, dependencies, tool_name,
			target_kind, target_id, base_revision, arguments, before_snapshot,
			proposed_snapshot, trace, risk_level, applied
		FROM assistant_change_operations
		WHERE change_set_id=$1
		ORDER BY position
		FOR UPDATE
	`, changeSetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var operations []assistant.ChangeOperation
	for rows.Next() {
		var operation assistant.ChangeOperation
		var dependencies []byte
		if err := rows.Scan(
			&operation.ID,
			&operation.Position,
			&operation.GroupKey,
			&operation.Selected,
			&dependencies,
			&operation.ToolName,
			&operation.TargetKind,
			&operation.TargetID,
			&operation.BaseRevision,
			&operation.Arguments,
			&operation.BeforeSnapshot,
			&operation.ProposedSnapshot,
			&operation.Trace,
			&operation.RiskLevel,
			&operation.Applied,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(dependencies, &operation.Dependencies); err != nil {
			return nil, err
		}
		operations = append(operations, operation)
	}
	return operations, rows.Err()
}

func selectAssistantGroups(
	operations []assistant.ChangeOperation,
	requested []string,
) (map[string]bool, error) {
	available := map[string]bool{}
	selected := map[string]bool{}
	for _, operation := range operations {
		available[operation.GroupKey] = true
		if len(requested) == 0 && operation.Selected {
			selected[operation.GroupKey] = true
		}
	}
	for _, group := range requested {
		if !available[group] {
			return nil, fmt.Errorf("unknown change group %q", group)
		}
		selected[group] = true
	}
	if len(selected) == 0 {
		return nil, errors.New("at least one change group must be selected")
	}
	for _, operation := range operations {
		if !selected[operation.GroupKey] {
			continue
		}
		for _, dependency := range operation.Dependencies {
			if !selected[dependency] {
				return nil, fmt.Errorf(
					"change group %q requires %q",
					operation.GroupKey,
					dependency,
				)
			}
		}
	}
	return selected, nil
}

func applyAssistantOperation(
	ctx context.Context,
	tx pgx.Tx,
	ownerID string,
	projectID string,
	runID string,
	changeSetID string,
	operationID string,
	operation assistant.ChangeOperation,
	annotationResolutionOverrides map[string]bool,
) error {
	switch operation.ToolName {
	case "document.insert", "document.replace_range", "document.append", "document.append_chunk",
		"document.apply_marks", "note.update", "annotation.propose_fixes", "document.prepare_replace_all",
		"document.prepare_transfer", "draft.prepare_transfer",
		"transcript.prepare_transfer", "version.restore":
		return applyAssistantDocument(
			ctx, tx, ownerID, projectID, runID, changeSetID, operationID, operation,
			annotationResolutionOverrides,
		)
	case "document.set_title", "tree.rename_node", "tree.set_status", "tree.move_node":
		return applyAssistantNodeMetadata(ctx, tx, ownerID, projectID, operation)
	case "tree.create_node", "document.create_draft", "note.create", "tree.duplicate_node":
		return applyAssistantNodeCreate(
			ctx, tx, ownerID, projectID, runID, changeSetID, operationID, operation,
		)
	case "dictionary.add", "dictionary.update", "dictionary.set_active":
		return applyAssistantDictionary(
			ctx, tx, ownerID, projectID, runID, operationID, operation,
		)
	case "assistant_guide.propose_create", "assistant_guide.propose_update":
		return applyAssistantGuide(ctx, tx, ownerID, projectID, runID, operation)
	case "project_memory.add", "project_memory.update", "project_memory.set_active",
		"project_memory.set_always_relevant":
		return applyAssistantMemory(
			ctx, tx, ownerID, projectID, runID, operationID, operation,
		)
	case "version.create_snapshot":
		return applyAssistantVersion(ctx, tx, ownerID, projectID, operation)
	case "canvas.propose_patch":
		return applyAssistantCanvas(
			ctx, tx, ownerID, projectID, runID, changeSetID, operationID, operation,
		)
	case "canvas.promote_cards":
		return applyAssistantCanvasPromotion(
			ctx, tx, ownerID, projectID, runID, changeSetID, operationID, operation,
		)
	case "tree.archive":
		if operation.TargetID == nil {
			return errors.New("archive target is missing")
		}
		command, err := tx.Exec(ctx, `
			UPDATE document_nodes n SET trashed_at=now(), updated_at=now()
			FROM projects p
			WHERE n.id=$1 AND n.project_id=$2 AND p.id=n.project_id
				AND p.owner_user_id=$3 AND n.trashed_at IS NULL
		`, *operation.TargetID, projectID, ownerID)
		if err != nil {
			return err
		}
		if command.RowsAffected() != 1 {
			return assistant.ErrStaleChangeSet
		}
		return nil
	default:
		return fmt.Errorf("tool %q cannot be committed by Assistant", operation.ToolName)
	}
}

func applyAssistantCanvas(
	ctx context.Context,
	tx pgx.Tx,
	ownerID string,
	projectID string,
	runID string,
	changeSetID string,
	operationID string,
	operation assistant.ChangeOperation,
) error {
	if operation.TargetID == nil || operation.BaseRevision == nil {
		return errors.New("canvas target and base revision are required")
	}
	references, err := canvasdocument.Validate(operation.ProposedSnapshot)
	if err != nil {
		return err
	}
	if err := validateCanvasResourcesTx(ctx, tx, ownerID, projectID, references); err != nil {
		return err
	}
	var current int
	if err := tx.QueryRow(ctx, `
		SELECT c.current_revision
		FROM canvases c
		JOIN projects p ON p.id=c.project_id
		WHERE c.id=$1 AND c.project_id=$2 AND p.owner_user_id=$3 AND c.trashed_at IS NULL
		FOR UPDATE OF c
	`, *operation.TargetID, projectID, ownerID).Scan(&current); err != nil {
		return err
	}
	if current != *operation.BaseRevision {
		return assistant.ErrStaleChangeSet
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO canvas_revisions(
			id, canvas_id, revision, base_revision, schema_version, content, source
		) VALUES($1,$2,$3,$4,$5,$6,'assistant')
	`, uuid.NewString(), *operation.TargetID, current+1, current,
		canvasdocument.SchemaVersion, operation.ProposedSnapshot); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE canvases SET current_revision=$2, updated_at=now() WHERE id=$1
	`, *operation.TargetID, current+1)
	_ = runID
	_ = changeSetID
	_ = operationID
	return err
}

type assistantCanvasPromotionTrace struct {
	Items []struct {
		NodeID     string  `json:"nodeId"`
		DocumentID string  `json:"documentId"`
		Title      string  `json:"title"`
		Kind       string  `json:"kind"`
		ParentID   *string `json:"parentId"`
	} `json:"items"`
}

func applyAssistantCanvasPromotion(
	ctx context.Context,
	tx pgx.Tx,
	ownerID string,
	projectID string,
	runID string,
	changeSetID string,
	operationID string,
	operation assistant.ChangeOperation,
) error {
	if operation.TargetID == nil || operation.BaseRevision == nil {
		return errors.New("canvas promotion target and base revision are required")
	}
	var trace assistantCanvasPromotionTrace
	if err := json.Unmarshal(operation.Trace, &trace); err != nil || len(trace.Items) == 0 || len(trace.Items) > 50 {
		return errors.New("canvas promotion trace is invalid")
	}
	var current int
	var currentContent, structureJSON json.RawMessage
	if err := tx.QueryRow(ctx, `
		SELECT c.current_revision, r.content, p.structure_schema
		FROM canvases c
		JOIN canvas_revisions r ON r.canvas_id=c.id AND r.revision=c.current_revision
		JOIN projects p ON p.id=c.project_id
		WHERE c.id=$1 AND c.project_id=$2 AND p.owner_user_id=$3
			AND c.trashed_at IS NULL AND p.status <> 'trash'
		FOR UPDATE OF c, p
	`, *operation.TargetID, projectID, ownerID).Scan(&current, &currentContent, &structureJSON); err != nil {
		return err
	}
	if current != *operation.BaseRevision {
		return assistant.ErrStaleChangeSet
	}
	var structure domain.ProjectStructure
	if err := json.Unmarshal(structureJSON, &structure); err != nil {
		return err
	}
	content := append(json.RawMessage(nil), currentContent...)
	for _, item := range trace.Items {
		if strings.TrimSpace(item.Title) == "" || len([]rune(item.Title)) > 200 ||
			strings.TrimSpace(item.Kind) == "" {
			return errors.New("canvas promotion metadata is invalid")
		}
		var parentKind *string
		if item.ParentID != nil {
			var value string
			if err := tx.QueryRow(ctx, `
				SELECT kind FROM document_nodes
				WHERE id=$1 AND project_id=$2 AND trashed_at IS NULL
				FOR UPDATE
			`, *item.ParentID, projectID).Scan(&value); err != nil {
				return err
			}
			parentKind = &value
		}
		if err := validateNodePlacement(structure, item.Kind, parentKind); err != nil {
			return err
		}
		contentBearing := false
		for _, nodeType := range structure.NodeTypes {
			if nodeType.Key == item.Kind {
				contentBearing = nodeType.ContentBearing
				break
			}
		}
		if !contentBearing {
			return ErrInvalidParentType
		}
		promotion, err := canvasdocument.PromoteNode(
			content, item.NodeID, *operation.TargetID, item.DocumentID,
		)
		if err != nil {
			return err
		}
		content = promotion.Content
		parsed, err := markdowntree.ParseMarkdown(
			"canvas-assistant-promotion.md", []byte(promotion.SourceText), nil,
		)
		if err != nil {
			return err
		}
		var position float64
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(MAX(position), 0)::float8 + 1000
			FROM document_nodes
			WHERE project_id=$1 AND parent_id IS NOT DISTINCT FROM $2
		`, projectID, item.ParentID).Scan(&position); err != nil {
			return err
		}
		frontmatter := fmt.Sprintf(
			"---\nwriters_studio_canvas_id: %s\nwriters_studio_canvas_node_id: %s\n---\n",
			*operation.TargetID, item.NodeID,
		)
		if _, err := tx.Exec(ctx, `
			INSERT INTO document_nodes(
				id, project_id, parent_id, kind, title, position, current_revision, source_frontmatter
			) VALUES($1,$2,$3,$4,$5,$6,1,$7)
		`, item.DocumentID, projectID, item.ParentID, item.Kind, item.Title, position, frontmatter); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO document_revisions(
				id, node_id, revision, base_revision, schema_version, content, plain_text,
				source, operation_id, assistant_run_id, assistant_change_set_id
			) VALUES($1,$2,1,0,3,$3,$4,'assistant',$5,$6,$7)
		`, uuid.NewString(), item.DocumentID, parsed.Content, parsed.PlainText,
			operationID, runID, changeSetID); err != nil {
			return err
		}
	}
	if !jsonEqual(content, operation.ProposedSnapshot) {
		return errors.New("canvas promotion proposal does not match typed operations")
	}
	references, err := canvasdocument.Validate(content)
	if err != nil {
		return err
	}
	if err := validateCanvasResourcesTx(ctx, tx, ownerID, projectID, references); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO canvas_revisions(
			id, canvas_id, revision, base_revision, schema_version, content, source
		) VALUES($1,$2,$3,$4,$5,$6,'assistant_promotion')
	`, uuid.NewString(), *operation.TargetID, current+1, current,
		references.SchemaVersion, content); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE canvases SET current_revision=$2, updated_at=now() WHERE id=$1
	`, *operation.TargetID, current+1)
	return err
}

func applyAssistantDocument(
	ctx context.Context,
	tx pgx.Tx,
	ownerID string,
	projectID string,
	runID string,
	changeSetID string,
	operationID string,
	operation assistant.ChangeOperation,
	annotationResolutionOverrides map[string]bool,
) error {
	if operation.TargetID == nil || operation.BaseRevision == nil {
		return errors.New("document target and base revision are required")
	}
	if err := editor.Validate(operation.ProposedSnapshot); err != nil {
		return err
	}
	plainText, err := editor.PlainText(operation.ProposedSnapshot)
	if err != nil {
		return err
	}
	var currentRevision int
	var currentContent json.RawMessage
	if err := tx.QueryRow(ctx, `
		SELECT n.current_revision, r.content
		FROM document_nodes n
		JOIN projects p ON p.id=n.project_id
		JOIN document_revisions r
			ON r.node_id=n.id AND r.revision=n.current_revision
		WHERE n.id=$1 AND n.project_id=$2 AND p.owner_user_id=$3 AND n.trashed_at IS NULL
		FOR UPDATE OF n
	`, *operation.TargetID, projectID, ownerID).Scan(
		&currentRevision,
		&currentContent,
	); err != nil {
		return err
	}
	if currentRevision != *operation.BaseRevision {
		return assistant.ErrStaleChangeSet
	}
	if len(operation.Trace) > 0 && string(operation.Trace) != "{}" {
		var wrapped struct {
			ResolvedTarget *assistant.ResolvedTarget `json:"resolvedTarget"`
		}
		if err := json.Unmarshal(operation.Trace, &wrapped); err != nil {
			return err
		}
		target := wrapped.ResolvedTarget
		if target == nil {
			var direct assistant.ResolvedTarget
			if err := json.Unmarshal(operation.Trace, &direct); err != nil {
				return err
			}
			if direct.NodeID != "" {
				target = &direct
			}
		}
		if target != nil {
			if err := assistant.ValidateProposalScope(
				currentContent,
				operation.ProposedSnapshot,
				*target,
			); err != nil {
				return err
			}
		}
	}
	nextRevision := currentRevision + 1
	edits, resolveAnnotations, annotationExpectations, err := assistantAnnotationEdits(operation)
	if err != nil {
		return err
	}
	for annotationID, resolve := range annotationResolutionOverrides {
		if _, exists := annotationExpectations[annotationID]; exists {
			resolveAnnotations[annotationID] = resolve
		}
	}
	schemaVersion, err := editor.SchemaVersion(operation.ProposedSnapshot)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO document_revisions(
			id, node_id, revision, base_revision, schema_version, content, plain_text,
			source, operation_id, assistant_run_id, assistant_change_set_id
		)
		VALUES($1,$2,$3,$4,$5,$6,$7,'assistant',$8,$9,$10)
	`, uuid.NewString(), *operation.TargetID, nextRevision, currentRevision,
		schemaVersion, operation.ProposedSnapshot, plainText, operationID, runID, changeSetID); err != nil {
		return err
	}
	if err := remapAssistantAnnotationsTx(
		ctx,
		tx,
		*operation.TargetID,
		operation.ProposedSnapshot,
		edits,
		resolveAnnotations,
		annotationExpectations,
		assistantAnnotationMutation{
			RunID: runID, ChangeSetID: changeSetID, ChangeOperationID: operation.ID,
			OperationID: operationID, DocumentRevision: nextRevision,
		},
	); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE document_nodes
		SET current_revision=$2, updated_at=now()
		WHERE id=$1
	`, *operation.TargetID, nextRevision)
	return err
}

func assistantAnnotationEdits(
	operation assistant.ChangeOperation,
) ([]documentannotation.TextEdit, map[string]bool, map[string]assistantAnnotationExpectation, error) {
	result := make([]documentannotation.TextEdit, 0)
	resolve := map[string]bool{}
	expectations := map[string]assistantAnnotationExpectation{}
	if operation.ToolName != "document.replace_range" &&
		operation.ToolName != "annotation.propose_fixes" {
		return result, resolve, expectations, nil
	}
	var arguments struct {
		Edits []struct {
			BlockID      string `json:"blockId"`
			Start        int    `json:"start"`
			End          int    `json:"end"`
			ExpectedText string `json:"expectedText"`
			Replacement  string `json:"replacement"`
		} `json:"edits"`
		AnnotationFixes []struct {
			AnnotationID       string `json:"annotationId"`
			AnnotationRevision int    `json:"annotationRevision"`
			OriginalText       string `json:"originalText"`
			Resolve            bool   `json:"resolve"`
		} `json:"annotationFixes"`
	}
	if err := json.Unmarshal(operation.Arguments, &arguments); err != nil {
		return nil, nil, nil, err
	}
	for _, edit := range arguments.Edits {
		result = append(result, documentannotation.TextEdit{
			BlockID: edit.BlockID, Start: edit.Start, End: edit.End,
			Expected: edit.ExpectedText, Replacement: edit.Replacement,
		})
	}
	for _, fix := range arguments.AnnotationFixes {
		if fix.AnnotationID == "" || fix.AnnotationRevision < 1 || fix.OriginalText == "" {
			return nil, nil, nil, errors.New("annotation fix has invalid identity")
		}
		expectations[fix.AnnotationID] = assistantAnnotationExpectation{
			Revision: fix.AnnotationRevision, ExactText: fix.OriginalText,
		}
		if fix.Resolve {
			resolve[fix.AnnotationID] = true
		}
	}
	return result, resolve, expectations, nil
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

func applyAssistantNodeMetadata(
	ctx context.Context,
	tx pgx.Tx,
	ownerID string,
	projectID string,
	operation assistant.ChangeOperation,
) error {
	if operation.TargetID == nil {
		return errors.New("node target is required")
	}
	var proposal nodeProposal
	if err := json.Unmarshal(operation.ProposedSnapshot, &proposal); err != nil {
		return err
	}
	var currentKind string
	var structureJSON []byte
	if err := tx.QueryRow(ctx, `
		SELECT n.kind, p.structure_schema
		FROM document_nodes n
		JOIN projects p ON p.id=n.project_id
		WHERE n.id=$1 AND n.project_id=$2 AND p.owner_user_id=$3 AND n.trashed_at IS NULL
		FOR UPDATE OF n
	`, *operation.TargetID, projectID, ownerID).Scan(&currentKind, &structureJSON); err != nil {
		return err
	}
	if operation.ToolName == "tree.move_node" {
		var structure domain.ProjectStructure
		if err := json.Unmarshal(structureJSON, &structure); err != nil {
			return err
		}
		var parentKind *string
		if proposal.ParentID != nil {
			if *proposal.ParentID == *operation.TargetID {
				return ErrInvalidDocumentMove
			}
			var value string
			if err := tx.QueryRow(ctx, `
				SELECT kind FROM document_nodes
				WHERE id=$1 AND project_id=$2 AND trashed_at IS NULL
			`, *proposal.ParentID, projectID).Scan(&value); err != nil {
				return err
			}
			parentKind = &value
			var createsCycle bool
			if err := tx.QueryRow(ctx, `
				WITH RECURSIVE descendants(id) AS (
					SELECT id
					FROM document_nodes
					WHERE id=$1 AND project_id=$2
					UNION
					SELECT child.id
					FROM document_nodes child
					JOIN descendants parent ON child.parent_id=parent.id
					WHERE child.project_id=$2
				)
				SELECT EXISTS(SELECT 1 FROM descendants WHERE id=$3)
			`, *operation.TargetID, projectID, *proposal.ParentID).Scan(&createsCycle); err != nil {
				return err
			}
			if createsCycle {
				return ErrInvalidDocumentMove
			}
		}
		if err := validateNodePlacement(structure, currentKind, parentKind); err != nil {
			return err
		}
	}
	command, err := tx.Exec(ctx, `
		UPDATE document_nodes
		SET
			title=CASE WHEN $4='document.set_title' OR $4='tree.rename_node' THEN $5 ELSE title END,
			status=CASE WHEN $4='tree.set_status' THEN $6 ELSE status END,
			parent_id=CASE WHEN $4='tree.move_node' THEN $7 ELSE parent_id END,
			position=CASE WHEN $4='tree.move_node' THEN $8 ELSE position END,
			updated_at=now()
		WHERE id=$1 AND project_id=$2
			AND EXISTS(SELECT 1 FROM projects WHERE id=$2 AND owner_user_id=$3)
	`, *operation.TargetID, projectID, ownerID, operation.ToolName, proposal.Title,
		proposal.Status, proposal.ParentID, proposal.Position)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func applyAssistantNodeCreate(
	ctx context.Context,
	tx pgx.Tx,
	ownerID string,
	projectID string,
	runID string,
	changeSetID string,
	operationID string,
	operation assistant.ChangeOperation,
) error {
	var proposal nodeProposal
	if err := json.Unmarshal(operation.ProposedSnapshot, &proposal); err != nil {
		return err
	}
	if proposal.ID == "" || strings.TrimSpace(proposal.Kind) == "" ||
		strings.TrimSpace(proposal.Title) == "" {
		return errors.New("invalid node proposal")
	}
	var structureJSON []byte
	if err := tx.QueryRow(ctx, `
		SELECT structure_schema
		FROM projects
		WHERE id=$1 AND owner_user_id=$2
		FOR UPDATE
	`, projectID, ownerID).Scan(&structureJSON); err != nil {
		return err
	}
	var structure domain.ProjectStructure
	if err := json.Unmarshal(structureJSON, &structure); err != nil {
		return err
	}
	var parentKind *string
	if proposal.ParentID != nil {
		var value string
		if err := tx.QueryRow(ctx, `
			SELECT kind FROM document_nodes
			WHERE id=$1 AND project_id=$2 AND trashed_at IS NULL
		`, *proposal.ParentID, projectID).Scan(&value); err != nil {
			return err
		}
		parentKind = &value
	}
	if err := validateNodePlacement(structure, proposal.Kind, parentKind); err != nil {
		return err
	}
	contentBearing := false
	for _, nodeType := range structure.NodeTypes {
		if nodeType.Key == proposal.Kind {
			contentBearing = nodeType.ContentBearing
			break
		}
	}
	currentRevision := 0
	if contentBearing {
		currentRevision = 1
		if len(proposal.Content) == 0 {
			return errors.New("content-bearing node has no document")
		}
		if err := editor.Validate(proposal.Content); err != nil {
			return err
		}
	}
	if proposal.Position == 0 {
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(max(position), 0)::float8 + 1000
			FROM document_nodes
			WHERE project_id=$1 AND parent_id IS NOT DISTINCT FROM $2
		`, projectID, proposal.ParentID).Scan(&proposal.Position); err != nil {
			return err
		}
	}
	if proposal.Status == "" {
		proposal.Status = "draft"
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO document_nodes(
			id, project_id, parent_id, kind, title, status, position, current_revision
		)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)
	`, proposal.ID, projectID, proposal.ParentID, proposal.Kind, proposal.Title,
		proposal.Status, proposal.Position, currentRevision); err != nil {
		return err
	}
	if !contentBearing {
		return nil
	}
	plainText, err := editor.PlainText(proposal.Content)
	if err != nil {
		return err
	}
	schemaVersion, err := editor.SchemaVersion(proposal.Content)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO document_revisions(
			id, node_id, revision, base_revision, schema_version, content, plain_text,
			source, operation_id, assistant_run_id, assistant_change_set_id
		)
		VALUES($1,$2,1,0,$3,$4,$5,'assistant',$6,$7,$8)
	`, uuid.NewString(), proposal.ID, schemaVersion, proposal.Content, plainText, operationID, runID,
		changeSetID)
	return err
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

func applyAssistantDictionary(
	ctx context.Context,
	tx pgx.Tx,
	ownerID string,
	projectID string,
	runID string,
	operationID string,
	operation assistant.ChangeOperation,
) error {
	var proposal dictionaryProposal
	if err := json.Unmarshal(operation.ProposedSnapshot, &proposal); err != nil {
		return err
	}
	variants, err := json.Marshal(proposal.Variants)
	if err != nil {
		return err
	}
	if operation.ToolName == "dictionary.add" {
		if proposal.ID == "" {
			return errors.New("dictionary proposal has no ID")
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO dictionary_terms(
				id, project_id, canonical, variants, kind, language, pronunciation,
				active, revision, source, operation_id, assistant_run_id
			)
			SELECT $1,p.id,$3,$4,$5,$6,$7,$8,1,'assistant',$9,$10
			FROM projects p
			WHERE p.id=$2 AND p.owner_user_id=$11
		`, proposal.ID, projectID, proposal.Canonical, variants, proposal.Kind,
			proposal.Language, proposal.Pronunciation, proposal.Active, operationID,
			runID, ownerID)
		return err
	}
	if operation.TargetID == nil || operation.BaseRevision == nil {
		return errors.New("dictionary target and base revision are required")
	}
	command, err := tx.Exec(ctx, `
		UPDATE dictionary_terms d
		SET canonical=$4, variants=$5, kind=$6, language=$7, pronunciation=$8,
			active=$9, revision=revision+1, source='assistant',
			operation_id=$10, assistant_run_id=$11, updated_at=now()
		FROM projects p
		WHERE d.id=$1 AND d.project_id=$2 AND p.id=d.project_id
			AND p.owner_user_id=$3 AND d.revision=$12
	`, *operation.TargetID, projectID, ownerID, proposal.Canonical, variants,
		proposal.Kind, proposal.Language, proposal.Pronunciation, proposal.Active,
		operationID, runID, *operation.BaseRevision)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return assistant.ErrStaleChangeSet
	}
	return nil
}

type guideProposal struct {
	Content      string `json:"content"`
	BaseRevision int    `json:"baseRevision"`
}

func applyAssistantGuide(
	ctx context.Context,
	tx pgx.Tx,
	ownerID string,
	projectID string,
	runID string,
	operation assistant.ChangeOperation,
) error {
	var proposal guideProposal
	if err := json.Unmarshal(operation.ProposedSnapshot, &proposal); err != nil {
		return err
	}
	if err := assistant.ValidateGuide(proposal.Content); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO assistant_guides(
			project_id, current_revision, updated_by_user_id, active
		)
		VALUES($1,0,$2,true)
		ON CONFLICT(project_id) DO NOTHING
	`, projectID, ownerID); err != nil {
		return err
	}
	var nextRevision int
	command, err := tx.Exec(ctx, `
		UPDATE assistant_guides
		SET current_revision=current_revision+1, updated_by_user_id=$2,
			active=true, updated_at=now()
		WHERE project_id=$1 AND current_revision=$3
	`, projectID, ownerID, proposal.BaseRevision)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return assistant.ErrStaleChangeSet
	}
	nextRevision = proposal.BaseRevision + 1
	_, err = tx.Exec(ctx, `
		INSERT INTO assistant_guide_revisions(
			id, project_id, revision, content, source, assistant_run_id, created_by_user_id
		)
		VALUES($1,$2,$3,$4,'assistant',$5,$6)
	`, uuid.NewString(), projectID, nextRevision, proposal.Content, runID, ownerID)
	return err
}

func applyAssistantMemory(
	ctx context.Context,
	tx pgx.Tx,
	ownerID string,
	projectID string,
	runID string,
	operationID string,
	operation assistant.ChangeOperation,
) error {
	var proposal assistant.MemoryRecord
	if err := json.Unmarshal(operation.ProposedSnapshot, &proposal); err != nil {
		return err
	}
	proposal.ProjectID = projectID
	proposal.Source = "assistant"
	proposal.OperationID = &operationID
	proposal.AssistantRunID = &runID
	if err := proposal.Validate(); err != nil {
		return err
	}
	references, err := json.Marshal(proposal.SourceReferences)
	if err != nil {
		return err
	}
	tags, err := json.Marshal(proposal.Tags)
	if err != nil {
		return err
	}
	if operation.ToolName == "project_memory.add" {
		if proposal.ID == "" {
			return errors.New("memory proposal has no ID")
		}
		proposal.Revision = 1
		_, err := tx.Exec(ctx, `
			INSERT INTO project_memory_records(
				id, project_id, kind, statement, scope, scope_node_id,
				source_references, tags, active, always_relevant, revision,
				source, operation_id, assistant_run_id, created_by_user_id
			)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,1,'assistant',$11,$12,$13)
		`, proposal.ID, projectID, proposal.Kind, proposal.Statement, proposal.Scope,
			proposal.ScopeNodeID, references, tags, proposal.Active,
			proposal.AlwaysRelevant, operationID, runID, ownerID)
		if err != nil {
			return err
		}
	} else {
		if operation.TargetID == nil || operation.BaseRevision == nil {
			return errors.New("memory target and base revision are required")
		}
		proposal.ID = *operation.TargetID
		proposal.Revision = *operation.BaseRevision + 1
		command, err := tx.Exec(ctx, `
			UPDATE project_memory_records
			SET kind=$4, statement=$5, scope=$6, scope_node_id=$7,
				source_references=$8, tags=$9, active=$10, always_relevant=$11,
				revision=$12, source='assistant', operation_id=$13,
				assistant_run_id=$14, updated_at=now()
			WHERE id=$1 AND project_id=$2 AND revision=$3
		`, proposal.ID, projectID, *operation.BaseRevision, proposal.Kind,
			proposal.Statement, proposal.Scope, proposal.ScopeNodeID, references, tags,
			proposal.Active, proposal.AlwaysRelevant, proposal.Revision, operationID,
			runID)
		if err != nil {
			return err
		}
		if command.RowsAffected() == 0 {
			return assistant.ErrStaleChangeSet
		}
	}
	snapshot, err := json.Marshal(proposal)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO project_memory_revisions(
			id, memory_id, revision, snapshot, source, operation_id, assistant_run_id
		)
		VALUES($1,$2,$3,$4,'assistant',$5,$6)
	`, uuid.NewString(), proposal.ID, proposal.Revision, snapshot, operationID, runID)
	return err
}

type versionProposal struct {
	ID     string `json:"id"`
	NodeID string `json:"nodeId"`
	Name   string `json:"name"`
	Note   string `json:"note"`
}

func applyAssistantVersion(
	ctx context.Context,
	tx pgx.Tx,
	ownerID string,
	projectID string,
	operation assistant.ChangeOperation,
) error {
	var proposal versionProposal
	if err := json.Unmarshal(operation.ProposedSnapshot, &proposal); err != nil {
		return err
	}
	if proposal.ID == "" || proposal.NodeID == "" || strings.TrimSpace(proposal.Name) == "" {
		return errors.New("invalid version proposal")
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO named_versions(id, node_id, revision, name, note)
		SELECT $1,n.id,n.current_revision,$4,$5
		FROM document_nodes n
		JOIN projects p ON p.id=n.project_id
		WHERE n.id=$2 AND n.project_id=$3 AND p.owner_user_id=$6
	`, proposal.ID, proposal.NodeID, projectID, proposal.Name, proposal.Note, ownerID)
	return err
}

func undoAssistantOperation(
	ctx context.Context,
	tx pgx.Tx,
	ownerID string,
	projectID string,
	runID string,
	changeSetID string,
	operationID string,
	operation assistant.ChangeOperation,
) error {
	switch operation.ToolName {
	case "document.insert", "document.replace_range", "document.append", "document.append_chunk",
		"document.apply_marks", "note.update", "annotation.propose_fixes", "document.prepare_replace_all",
		"document.prepare_transfer", "draft.prepare_transfer",
		"transcript.prepare_transfer", "version.restore":
		var before domain.DocumentNode
		if err := json.Unmarshal(operation.BeforeSnapshot, &before); err != nil {
			return err
		}
		if operation.TargetID == nil || len(before.Content) == 0 {
			return errors.New("document undo snapshot is missing")
		}
		plainText, err := editor.PlainText(before.Content)
		if err != nil {
			return err
		}
		schemaVersion, err := editor.SchemaVersion(before.Content)
		if err != nil {
			return err
		}
		var current int
		if err := tx.QueryRow(ctx, `
			SELECT current_revision FROM document_nodes
			WHERE id=$1 AND project_id=$2
			FOR UPDATE
		`, *operation.TargetID, projectID).Scan(&current); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO document_revisions(
				id, node_id, revision, base_revision, schema_version, content, plain_text,
				source, operation_id, assistant_run_id, assistant_change_set_id
			)
			VALUES($1,$2,$3,$4,$5,$6,$7,'assistant_undo',$8,$9,$10)
		`, uuid.NewString(), *operation.TargetID, current+1, current, schemaVersion, before.Content,
			plainText, operationID, runID, changeSetID); err != nil {
			return err
		}
		if err := undoAssistantAnnotationsTx(
			ctx,
			tx,
			*operation.TargetID,
			operation.ID,
			assistantAnnotationMutation{
				RunID: runID, ChangeSetID: changeSetID, ChangeOperationID: operation.ID,
				OperationID: operationID, DocumentRevision: current + 1,
			},
		); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE document_nodes SET current_revision=$2, updated_at=now() WHERE id=$1
		`, *operation.TargetID, current+1)
		return err
	case "document.set_title", "tree.rename_node", "tree.set_status", "tree.move_node":
		var before domain.DocumentNode
		if err := json.Unmarshal(operation.BeforeSnapshot, &before); err != nil {
			return err
		}
		if operation.TargetID == nil {
			return errors.New("node undo target is missing")
		}
		_, err := tx.Exec(ctx, `
			UPDATE document_nodes
			SET title=$2, status=$3, parent_id=$4, position=$5, updated_at=now()
			WHERE id=$1 AND project_id=$6
		`, *operation.TargetID, before.Title, before.Status, before.ParentID,
			before.Position, projectID)
		return err
	case "tree.create_node", "document.create_draft", "note.create", "tree.duplicate_node":
		var proposal nodeProposal
		if err := json.Unmarshal(operation.ProposedSnapshot, &proposal); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			UPDATE document_nodes SET trashed_at=now(), updated_at=now()
			WHERE id=$1 AND project_id=$2
		`, proposal.ID, projectID)
		return err
	case "dictionary.add":
		var proposal dictionaryProposal
		if err := json.Unmarshal(operation.ProposedSnapshot, &proposal); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			UPDATE dictionary_terms
			SET active=false, revision=revision+1, source='assistant',
				operation_id=$3, assistant_run_id=$4, updated_at=now()
			WHERE id=$1 AND project_id=$2
		`, proposal.ID, projectID, operationID, runID)
		return err
	case "dictionary.update", "dictionary.set_active":
		var before domain.DictionaryTerm
		if err := json.Unmarshal(operation.BeforeSnapshot, &before); err != nil {
			return err
		}
		variants, _ := json.Marshal(before.Variants)
		_, err := tx.Exec(ctx, `
			UPDATE dictionary_terms
			SET canonical=$3, variants=$4, kind=$5, language=$6, pronunciation=$7,
				active=$8, revision=revision+1, source='assistant',
				operation_id=$9, assistant_run_id=$10, updated_at=now()
			WHERE id=$1 AND project_id=$2
		`, before.ID, projectID, before.Canonical, variants, before.Kind,
			before.Language, before.Pronunciation, before.Active, operationID, runID)
		return err
	case "assistant_guide.propose_create":
		_, err := tx.Exec(ctx, `
			UPDATE assistant_guides
			SET active=false, updated_by_user_id=$2, updated_at=now()
			WHERE project_id=$1
		`, projectID, ownerID)
		return err
	case "assistant_guide.propose_update":
		var before assistant.Guide
		if err := json.Unmarshal(operation.BeforeSnapshot, &before); err != nil {
			return err
		}
		var revision int
		if err := tx.QueryRow(ctx, `
			UPDATE assistant_guides
			SET current_revision=current_revision+1, active=true,
				updated_by_user_id=$2, updated_at=now()
			WHERE project_id=$1
			RETURNING current_revision
		`, projectID, ownerID).Scan(&revision); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO assistant_guide_revisions(
				id, project_id, revision, content, source, restored_from_revision,
				assistant_run_id, created_by_user_id
			)
			VALUES($1,$2,$3,$4,'restore',$5,$6,$7)
		`, uuid.NewString(), projectID, revision, before.Content, before.Revision,
			runID, ownerID)
		return err
	case "project_memory.add":
		var proposal assistant.MemoryRecord
		if err := json.Unmarshal(operation.ProposedSnapshot, &proposal); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			UPDATE project_memory_records
			SET active=false, revision=revision+1, source='assistant',
				operation_id=$3, assistant_run_id=$4, updated_at=now()
			WHERE id=$1 AND project_id=$2
		`, proposal.ID, projectID, operationID, runID)
		return err
	case "project_memory.update", "project_memory.set_active",
		"project_memory.set_always_relevant":
		var before assistant.MemoryRecord
		if err := json.Unmarshal(operation.BeforeSnapshot, &before); err != nil {
			return err
		}
		references, _ := json.Marshal(before.SourceReferences)
		tags, _ := json.Marshal(before.Tags)
		_, err := tx.Exec(ctx, `
			UPDATE project_memory_records
			SET kind=$3, statement=$4, scope=$5, scope_node_id=$6,
				source_references=$7, tags=$8, active=$9, always_relevant=$10,
				revision=revision+1, source='assistant', operation_id=$11,
				assistant_run_id=$12, updated_at=now()
			WHERE id=$1 AND project_id=$2
		`, before.ID, projectID, before.Kind, before.Statement, before.Scope,
			before.ScopeNodeID, references, tags, before.Active, before.AlwaysRelevant,
			operationID, runID)
		return err
	case "version.create_snapshot":
		var proposal versionProposal
		if err := json.Unmarshal(operation.ProposedSnapshot, &proposal); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			DELETE FROM named_versions v
			USING document_nodes n
			WHERE v.id=$1 AND n.id=v.node_id AND n.project_id=$2
		`, proposal.ID, projectID)
		return err
	case "canvas.propose_patch", "canvas.promote_cards":
		if operation.TargetID == nil {
			return errors.New("canvas undo target is missing")
		}
		references, err := canvasdocument.Validate(operation.BeforeSnapshot)
		if err != nil {
			return err
		}
		if err := validateCanvasResourcesTx(ctx, tx, ownerID, projectID, references); err != nil {
			return err
		}
		var current int
		if err := tx.QueryRow(ctx, `
			SELECT current_revision FROM canvases
			WHERE id=$1 AND project_id=$2 AND trashed_at IS NULL
			FOR UPDATE
		`, *operation.TargetID, projectID).Scan(&current); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO canvas_revisions(
				id, canvas_id, revision, base_revision, schema_version, content, source
			) VALUES($1,$2,$3,$4,$5,$6,'assistant_undo')
		`, uuid.NewString(), *operation.TargetID, current+1, current,
			canvasdocument.SchemaVersion, operation.BeforeSnapshot); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `
			UPDATE canvases SET current_revision=$2, updated_at=now() WHERE id=$1
		`, *operation.TargetID, current+1); err != nil {
			return err
		}
		if operation.ToolName == "canvas.promote_cards" {
			var trace assistantCanvasPromotionTrace
			if err := json.Unmarshal(operation.Trace, &trace); err != nil {
				return err
			}
			for _, item := range trace.Items {
				if _, err := tx.Exec(ctx, `
					UPDATE document_nodes SET trashed_at=now(), updated_at=now()
					WHERE id=$1 AND project_id=$2 AND trashed_at IS NULL
				`, item.DocumentID, projectID); err != nil {
					return err
				}
			}
		}
		return nil
	case "tree.archive":
		if operation.TargetID == nil {
			return errors.New("archive undo target is missing")
		}
		command, err := tx.Exec(ctx, `
			UPDATE document_nodes n SET trashed_at=NULL, updated_at=now()
			FROM projects p
			WHERE n.id=$1 AND n.project_id=$2 AND p.id=n.project_id
				AND p.owner_user_id=$3 AND n.trashed_at IS NOT NULL
		`, *operation.TargetID, projectID, ownerID)
		if err != nil {
			return err
		}
		if command.RowsAffected() != 1 {
			return assistant.ErrStaleChangeSet
		}
		return nil
	default:
		return fmt.Errorf("tool %q has no compensating undo", operation.ToolName)
	}
}

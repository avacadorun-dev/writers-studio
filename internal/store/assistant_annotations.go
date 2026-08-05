package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"writers.studio/internal/assistant"
	"writers.studio/internal/documentannotation"
	"writers.studio/internal/domain"
	"writers.studio/internal/editor"
)

type assistantAnnotationMutation struct {
	RunID             string
	ChangeSetID       string
	ChangeOperationID string
	OperationID       string
	DocumentRevision  int
}

type assistantAnnotationExpectation struct {
	Revision  int
	ExactText string
}

func remapAssistantAnnotationsTx(
	ctx context.Context,
	tx pgx.Tx,
	nodeID string,
	content json.RawMessage,
	edits []documentannotation.TextEdit,
	resolve map[string]bool,
	expectations map[string]assistantAnnotationExpectation,
	mutation assistantAnnotationMutation,
) error {
	annotations, err := annotationsForAssistantUpdate(ctx, tx, nodeID)
	if err != nil {
		return err
	}
	verified := make(map[string]bool, len(expectations))
	for _, before := range annotations {
		if expectation, exists := expectations[before.ID]; exists {
			if before.Revision != expectation.Revision ||
				before.Anchor.ExactText != expectation.ExactText || before.AnchorStatus != "attached" {
				return assistant.ErrStaleChangeSet
			}
			verified[before.ID] = true
		}
		mapped, detached, err := documentannotation.RemapAnchor(before.Anchor, edits)
		if err != nil {
			return err
		}
		if !detached {
			mapped, err = editor.AnchorAtRange(content, mapped.BlockID, mapped.Start, mapped.End)
			if err != nil || mapped.ExactText == "" {
				detached = true
			}
		}
		if resolve[before.ID] && detached {
			return errors.New("resolved annotation lost its exact anchor")
		}

		after := before
		after.BaseRevision = mutation.DocumentRevision
		after.Revision = before.Revision + 1
		after.UpdatedAt = time.Now()
		eventKind := "assistant_reanchor"
		if detached {
			after.AnchorStatus = "detached"
			eventKind = "assistant_detach"
		} else {
			after.Anchor = mapped
			after.AnchorStatus = "attached"
			after.Status = documentannotation.StatusAfterMapping(
				before.Status,
				before.Kind,
				before.DictionaryTermID != nil,
				false,
				true,
			)
		}
		if resolve[before.ID] {
			now := time.Now()
			after.Status = "resolved"
			after.ResolvedAt = &now
			eventKind = "assistant_resolve"
		}
		if err := updateAssistantAnnotationTx(ctx, tx, after); err != nil {
			return err
		}
		if err := insertAssistantAnnotationHistoryTx(
			ctx, tx, before, after, eventKind, "assistant", mutation,
		); err != nil {
			return err
		}
	}
	if len(verified) != len(expectations) {
		return assistant.ErrStaleChangeSet
	}
	return nil
}

func undoAssistantAnnotationsTx(
	ctx context.Context,
	tx pgx.Tx,
	nodeID string,
	changeOperationID string,
	mutation assistantAnnotationMutation,
) error {
	rows, err := tx.Query(ctx, `
		SELECT h.annotation_id::text,h.before_snapshot
		FROM document_annotation_history h
		JOIN document_annotations a ON a.id=h.annotation_id
		WHERE h.change_operation_id=$1 AND a.node_id=$2 AND h.source='assistant'
		ORDER BY h.annotation_revision
	`, changeOperationID, nodeID)
	if err != nil {
		return err
	}
	type restoreItem struct {
		annotationID string
		before       domain.DocumentAnnotation
	}
	items := make([]restoreItem, 0)
	for rows.Next() {
		var item restoreItem
		var snapshot []byte
		if err := rows.Scan(&item.annotationID, &snapshot); err != nil {
			rows.Close()
			return err
		}
		if err := json.Unmarshal(snapshot, &item.before); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, item := range items {
		current, err := annotationForAssistantUpdate(ctx, tx, item.annotationID, nodeID)
		if err != nil {
			return err
		}
		restored := item.before
		restored.Revision = current.Revision + 1
		restored.BaseRevision = mutation.DocumentRevision
		restored.UpdatedAt = time.Now()
		if err := updateAssistantAnnotationTx(ctx, tx, restored); err != nil {
			return err
		}
		if err := insertAssistantAnnotationHistoryTx(
			ctx, tx, current, restored, "assistant_undo", "assistant_undo", mutation,
		); err != nil {
			return err
		}
	}
	return nil
}

func annotationsForAssistantUpdate(
	ctx context.Context,
	tx pgx.Tx,
	nodeID string,
) ([]domain.DocumentAnnotation, error) {
	rows, err := tx.Query(ctx, annotationForAssistantSelect+`
		WHERE a.node_id=$1
		ORDER BY a.created_at
		FOR UPDATE OF a
	`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.DocumentAnnotation, 0)
	for rows.Next() {
		item, err := scanAssistantAnnotation(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func annotationForAssistantUpdate(
	ctx context.Context,
	tx pgx.Tx,
	annotationID, nodeID string,
) (domain.DocumentAnnotation, error) {
	return scanAssistantAnnotation(tx.QueryRow(ctx, annotationForAssistantSelect+`
		WHERE a.id=$1 AND a.node_id=$2
		FOR UPDATE OF a
	`, annotationID, nodeID))
}

const annotationForAssistantSelect = `
	SELECT a.id::text,a.project_id::text,a.node_id::text,n.title,a.kind,a.status,
		a.anchor_status,a.dictionary_term_id::text,a.block_id,a.start_offset,
		a.end_offset,a.exact_text,a.prefix,a.suffix,a.note,a.base_revision,a.revision,
		a.resolved_at,a.created_at,a.updated_at
	FROM document_annotations a
	JOIN document_nodes n ON n.id=a.node_id
`

func scanAssistantAnnotation(scanner rowScanner) (domain.DocumentAnnotation, error) {
	var item domain.DocumentAnnotation
	err := scanner.Scan(
		&item.ID, &item.ProjectID, &item.NodeID, &item.NodeTitle, &item.Kind,
		&item.Status, &item.AnchorStatus, &item.DictionaryTermID, &item.Anchor.BlockID,
		&item.Anchor.Start, &item.Anchor.End, &item.Anchor.ExactText, &item.Anchor.Prefix,
		&item.Anchor.Suffix, &item.Note, &item.BaseRevision, &item.Revision,
		&item.ResolvedAt, &item.CreatedAt, &item.UpdatedAt,
	)
	return item, err
}

func updateAssistantAnnotationTx(
	ctx context.Context,
	tx pgx.Tx,
	item domain.DocumentAnnotation,
) error {
	_, err := tx.Exec(ctx, `
		UPDATE document_annotations
		SET status=$2,anchor_status=$3,block_id=$4,start_offset=$5,end_offset=$6,
			exact_text=$7,prefix=$8,suffix=$9,base_revision=$10,revision=$11,
			resolved_at=$12,updated_at=$13
		WHERE id=$1
	`, item.ID, item.Status, item.AnchorStatus, item.Anchor.BlockID, item.Anchor.Start,
		item.Anchor.End, item.Anchor.ExactText, item.Anchor.Prefix, item.Anchor.Suffix,
		item.BaseRevision, item.Revision, item.ResolvedAt, item.UpdatedAt)
	return err
}

func insertAssistantAnnotationHistoryTx(
	ctx context.Context,
	tx pgx.Tx,
	before, after domain.DocumentAnnotation,
	eventKind, source string,
	mutation assistantAnnotationMutation,
) error {
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		return err
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO document_annotation_history(
			id,annotation_id,annotation_revision,event_kind,before_snapshot,after_snapshot,
			source,operation_id,change_operation_id,assistant_run_id,
			assistant_change_set_id,document_revision
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, uuid.NewString(), after.ID, after.Revision, eventKind, beforeJSON, afterJSON,
		source, mutation.OperationID, mutation.ChangeOperationID, mutation.RunID,
		mutation.ChangeSetID, mutation.DocumentRevision)
	return err
}

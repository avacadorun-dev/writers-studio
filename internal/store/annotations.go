package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"writers.studio/internal/documentannotation"
	"writers.studio/internal/domain"
	"writers.studio/internal/editor"
)

var ErrInvalidAnnotation = errors.New("invalid document annotation")

type CreateAnnotationInput struct {
	Kind             string
	DictionaryTermID *string
	Anchor           domain.AnnotationAnchor
	Note             string
	BaseRevision     int
}

type UpdateAnnotationInput struct {
	Note   *string
	Status *string
	Anchor *domain.AnnotationAnchor
}

type AnnotationAnchorUpdate struct {
	ID       string                  `json:"id"`
	Anchor   domain.AnnotationAnchor `json:"anchor"`
	Detached bool                    `json:"detached,omitempty"`
}

func (s *Store) ListProjectAnnotations(
	ctx context.Context,
	ownerID, projectID string,
) ([]domain.DocumentAnnotation, error) {
	return s.listAnnotations(ctx, ownerID, projectID, "")
}

func (s *Store) ListDocumentAnnotations(
	ctx context.Context,
	ownerID, nodeID string,
) ([]domain.DocumentAnnotation, error) {
	return s.listAnnotations(ctx, ownerID, "", nodeID)
}

func (s *Store) listAnnotations(
	ctx context.Context,
	ownerID, projectID, nodeID string,
) ([]domain.DocumentAnnotation, error) {
	var projectArg any
	if projectID != "" {
		projectArg = projectID
	}
	var nodeArg any
	if nodeID != "" {
		nodeArg = nodeID
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT a.id::text, a.project_id::text, a.node_id::text, n.title, a.kind,
			a.status, a.anchor_status,
			a.dictionary_term_id::text, a.block_id, a.start_offset, a.end_offset,
			a.exact_text, a.prefix, a.suffix, a.note, a.base_revision, a.revision, a.resolved_at,
			a.created_at, a.updated_at, a.target_snapshot,
			t.id::text, t.canonical, t.kind, t.language, t.active
		FROM document_annotations a
		JOIN document_nodes n ON n.id=a.node_id
		JOIN projects p ON p.id=a.project_id
		LEFT JOIN dictionary_terms t ON t.id=a.dictionary_term_id
		WHERE p.owner_user_id=$1
			AND n.trashed_at IS NULL
			AND ($2::uuid IS NULL OR a.project_id=$2::uuid)
			AND ($3::uuid IS NULL OR a.node_id=$3::uuid)
		ORDER BY
			CASE WHEN a.status='resolved' THEN 1 ELSE 0 END,
			a.updated_at DESC
	`, ownerID, projectArg, nodeArg)
	if err != nil {
		return nil, fmt.Errorf("list document annotations: %w", err)
	}
	defer rows.Close()
	result := make([]domain.DocumentAnnotation, 0)
	for rows.Next() {
		item, err := scanAnnotation(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) CreateDocumentAnnotation(
	ctx context.Context,
	ownerID, nodeID string,
	input CreateAnnotationInput,
) (domain.DocumentAnnotation, error) {
	input.Note = strings.TrimSpace(input.Note)
	if err := validateAnnotationInput(input); err != nil {
		return domain.DocumentAnnotation{}, err
	}
	id := uuid.NewString()
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var projectID string
		var currentRevision int
		var content json.RawMessage
		if err := tx.QueryRow(ctx, `
			SELECT n.project_id::text, n.current_revision, r.content
			FROM document_nodes n
			JOIN projects p ON p.id=n.project_id
			JOIN document_revisions r ON r.node_id=n.id AND r.revision=n.current_revision
			WHERE n.id=$1 AND p.owner_user_id=$2 AND n.trashed_at IS NULL
			FOR UPDATE OF n
		`, nodeID, ownerID).Scan(&projectID, &currentRevision, &content); err != nil {
			return err
		}
		if currentRevision != input.BaseRevision {
			return &RevisionConflictError{CurrentRevision: currentRevision}
		}
		if err := editor.ValidateAnchor(content, input.Anchor); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidAnnotation, err)
		}
		targetSnapshot := []byte(`{}`)
		if input.Kind == "dictionary_link" {
			if input.DictionaryTermID == nil {
				return ErrInvalidAnnotation
			}
			var canonical, kind, language string
			var active bool
			if err := tx.QueryRow(ctx, `
				SELECT canonical, kind, language, active
				FROM dictionary_terms
				WHERE id=$1 AND project_id=$2
			`, *input.DictionaryTermID, projectID).Scan(
				&canonical, &kind, &language, &active,
			); err != nil {
				return err
			}
			targetSnapshot, _ = json.Marshal(domain.AnnotationTerm{
				Canonical: canonical, Kind: kind, Language: language, Active: active,
			})
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO document_annotations(
				id, project_id, node_id, kind, dictionary_term_id, block_id,
				start_offset, end_offset, exact_text, prefix, suffix, note,
				target_snapshot, base_revision
			) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		`, id, projectID, nodeID, input.Kind, input.DictionaryTermID,
			input.Anchor.BlockID, input.Anchor.Start, input.Anchor.End,
			input.Anchor.ExactText, input.Anchor.Prefix, input.Anchor.Suffix,
			input.Note, targetSnapshot, currentRevision)
		return err
	})
	if err != nil {
		return domain.DocumentAnnotation{}, err
	}
	items, err := s.ListDocumentAnnotations(ctx, ownerID, nodeID)
	if err != nil {
		return domain.DocumentAnnotation{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return domain.DocumentAnnotation{}, pgx.ErrNoRows
}

func (s *Store) UpdateDocumentAnnotation(
	ctx context.Context,
	ownerID, annotationID string,
	input UpdateAnnotationInput,
) (domain.DocumentAnnotation, error) {
	if input.Note == nil && input.Status == nil && input.Anchor == nil {
		return domain.DocumentAnnotation{}, ErrInvalidAnnotation
	}
	if input.Note != nil {
		trimmed, err := documentannotation.NormalizeNote(*input.Note)
		if err != nil {
			return domain.DocumentAnnotation{}, ErrInvalidAnnotation
		}
		input.Note = &trimmed
	}
	if input.Status != nil && !validAnnotationStatus(*input.Status) {
		return domain.DocumentAnnotation{}, ErrInvalidAnnotation
	}
	var nodeID string
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var kind string
		var termID *string
		var currentRevision int
		var content json.RawMessage
		if err := tx.QueryRow(ctx, `
			SELECT a.node_id::text, a.kind, a.dictionary_term_id::text,
				n.current_revision, r.content
			FROM document_annotations a
			JOIN document_nodes n ON n.id=a.node_id
			JOIN projects p ON p.id=a.project_id
			JOIN document_revisions r ON r.node_id=n.id AND r.revision=n.current_revision
			WHERE a.id=$1 AND p.owner_user_id=$2
			FOR UPDATE OF a, n
		`, annotationID, ownerID).Scan(
			&nodeID, &kind, &termID, &currentRevision, &content,
		); err != nil {
			return err
		}
		if input.Note != nil && kind != "note" {
			return ErrInvalidAnnotation
		}
		if input.Status != nil {
			if err := documentannotation.ValidateRequestedStatus(
				kind, termID != nil, *input.Status,
			); err != nil {
				return ErrInvalidAnnotation
			}
		}
		if input.Anchor != nil {
			if err := editor.ValidateAnchor(content, *input.Anchor); err != nil {
				return fmt.Errorf("%w: %v", ErrInvalidAnnotation, err)
			}
		}
		resolvedAt := any(nil)
		if input.Status != nil && *input.Status == "resolved" {
			resolvedAt = time.Now()
		}
		_, err := tx.Exec(ctx, `
			UPDATE document_annotations
			SET note=COALESCE($2,note), status=COALESCE($3,status),
				block_id=COALESCE($4,block_id), start_offset=COALESCE($5,start_offset),
				end_offset=COALESCE($6,end_offset), exact_text=COALESCE($7,exact_text),
				prefix=COALESCE($8,prefix), suffix=COALESCE($9,suffix),
				base_revision=CASE WHEN $4::text IS NULL THEN base_revision ELSE $10 END,
				anchor_status=CASE WHEN $4::text IS NULL THEN anchor_status ELSE 'attached' END,
				resolved_at=CASE
					WHEN $3='resolved' THEN $11::timestamptz
					WHEN $3 IN ('open','needs_attention') THEN NULL
					ELSE resolved_at
				END,
				revision=revision+1, updated_at=now()
			WHERE id=$1
		`, annotationID, input.Note, input.Status,
			anchorString(input.Anchor, func(a domain.AnnotationAnchor) string { return a.BlockID }),
			anchorInt(input.Anchor, func(a domain.AnnotationAnchor) int { return a.Start }),
			anchorInt(input.Anchor, func(a domain.AnnotationAnchor) int { return a.End }),
			anchorString(input.Anchor, func(a domain.AnnotationAnchor) string { return a.ExactText }),
			anchorString(input.Anchor, func(a domain.AnnotationAnchor) string { return a.Prefix }),
			anchorString(input.Anchor, func(a domain.AnnotationAnchor) string { return a.Suffix }),
			currentRevision, resolvedAt)
		return err
	})
	if err != nil {
		return domain.DocumentAnnotation{}, err
	}
	items, err := s.ListDocumentAnnotations(ctx, ownerID, nodeID)
	if err != nil {
		return domain.DocumentAnnotation{}, err
	}
	for _, item := range items {
		if item.ID == annotationID {
			return item, nil
		}
	}
	return domain.DocumentAnnotation{}, pgx.ErrNoRows
}

func (s *Store) GetDocumentAnnotation(
	ctx context.Context,
	ownerID, annotationID string,
) (domain.DocumentAnnotation, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT a.id::text, a.project_id::text, a.node_id::text, n.title, a.kind,
			a.status, a.anchor_status, a.dictionary_term_id::text, a.block_id,
			a.start_offset, a.end_offset, a.exact_text, a.prefix, a.suffix, a.note,
			a.base_revision, a.revision, a.resolved_at, a.created_at, a.updated_at,
			a.target_snapshot, t.id::text, t.canonical, t.kind, t.language, t.active
		FROM document_annotations a
		JOIN document_nodes n ON n.id=a.node_id
		JOIN projects p ON p.id=a.project_id
		LEFT JOIN dictionary_terms t ON t.id=a.dictionary_term_id
		WHERE a.id=$1 AND p.owner_user_id=$2 AND n.trashed_at IS NULL
	`, annotationID, ownerID)
	return scanAnnotation(row)
}

func (s *Store) ListDocumentAnnotationHistory(
	ctx context.Context,
	ownerID, annotationID string,
) ([]domain.DocumentAnnotationHistory, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT h.id::text,h.annotation_id::text,h.annotation_revision,h.event_kind,
			h.before_snapshot,h.after_snapshot,h.source,h.operation_id::text,
			h.change_operation_id::text,
			h.assistant_run_id::text,h.assistant_change_set_id::text,
			h.document_revision,h.created_at
		FROM document_annotation_history h
		JOIN document_annotations a ON a.id=h.annotation_id
		JOIN projects p ON p.id=a.project_id
		WHERE h.annotation_id=$1 AND p.owner_user_id=$2
		ORDER BY h.annotation_revision DESC
	`, annotationID, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.DocumentAnnotationHistory, 0)
	for rows.Next() {
		var item domain.DocumentAnnotationHistory
		var before, after []byte
		if err := rows.Scan(
			&item.ID, &item.AnnotationID, &item.AnnotationRevision, &item.EventKind,
			&before, &after, &item.Source, &item.OperationID, &item.ChangeOperationID, &item.AssistantRunID,
			&item.AssistantChangeSetID, &item.DocumentRevision, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(before, &item.Before); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(after, &item.After); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) DeleteDocumentAnnotation(ctx context.Context, ownerID, annotationID string) error {
	tag, err := s.Pool.Exec(ctx, `
		DELETE FROM document_annotations a
		USING projects p
		WHERE a.id=$1 AND p.id=a.project_id AND p.owner_user_id=$2
	`, annotationID, ownerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func applyAnnotationAnchorsTx(
	ctx context.Context,
	tx pgx.Tx,
	nodeID string,
	nextRevision int,
	content json.RawMessage,
	updates []AnnotationAnchorUpdate,
) error {
	provided := make(map[string]bool, len(updates))
	for _, update := range updates {
		if update.ID == "" || provided[update.ID] {
			return ErrInvalidAnnotation
		}
		provided[update.ID] = true
		var currentStatus, kind string
		var termID *string
		if err := tx.QueryRow(ctx, `
			SELECT status, kind, dictionary_term_id::text
			FROM document_annotations
			WHERE id=$1 AND node_id=$2
			FOR UPDATE
		`, update.ID, nodeID).Scan(&currentStatus, &kind, &termID); err != nil {
			return err
		}
		anchorValid := editor.ValidateAnchor(content, update.Anchor) == nil
		nextStatus := documentannotation.StatusAfterMapping(
			currentStatus, kind, termID != nil, update.Detached, anchorValid,
		)
		if update.Detached || !anchorValid {
			if _, err := tx.Exec(ctx, `
				UPDATE document_annotations
				SET anchor_status='detached',
					base_revision=$2, revision=revision+1, updated_at=now()
				WHERE id=$1
			`, update.ID, nextRevision); err != nil {
				return err
			}
			continue
		}
		_, err := tx.Exec(ctx, `
		UPDATE document_annotations
		SET block_id=$2,start_offset=$3,end_offset=$4,exact_text=$5,prefix=$6,suffix=$7,
			base_revision=$8,
			status=$9, anchor_status='attached', revision=revision+1,
				updated_at=now()
			WHERE id=$1
		`, update.ID, update.Anchor.BlockID, update.Anchor.Start, update.Anchor.End,
			update.Anchor.ExactText, update.Anchor.Prefix, update.Anchor.Suffix,
			nextRevision, nextStatus)
		if err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, `
		UPDATE document_annotations
		SET anchor_status='detached', base_revision=$2, revision=revision+1, updated_at=now()
		WHERE node_id=$1 AND NOT (id::text = ANY($3::text[]))
	`, nodeID, nextRevision, mapKeys(provided))
	return err
}

func copyAnnotationsTx(
	ctx context.Context,
	tx pgx.Tx,
	fromNodeID, toNodeID, projectID string,
	baseRevision int,
	content json.RawMessage,
	updates []AnnotationAnchorUpdate,
	preserveUnmapped bool,
) error {
	rows, err := tx.Query(ctx, `
		SELECT id::text, kind, dictionary_term_id::text, status, block_id, start_offset,
			end_offset, exact_text, prefix, suffix, note, target_snapshot, resolved_at
		FROM document_annotations WHERE node_id=$1
	`, fromNodeID)
	if err != nil {
		return err
	}
	type copied struct {
		id, kind, status, blockID, exact, prefix, suffix, note string
		termID                                                 *string
		start, end                                             int
		snapshot                                               []byte
		resolvedAt                                             *time.Time
	}
	items := make([]copied, 0)
	for rows.Next() {
		var item copied
		if err := rows.Scan(&item.id, &item.kind, &item.termID, &item.status, &item.blockID,
			&item.start, &item.end, &item.exact, &item.prefix, &item.suffix, &item.note,
			&item.snapshot, &item.resolvedAt); err != nil {
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
	byID := make(map[string]AnnotationAnchorUpdate, len(updates))
	for _, update := range updates {
		byID[update.ID] = update
	}
	for _, item := range items {
		anchor := domain.AnnotationAnchor{BlockID: item.blockID, Start: item.start, End: item.end,
			ExactText: item.exact, Prefix: item.prefix, Suffix: item.suffix}
		status := item.status
		mapped := false
		if update, ok := byID[item.id]; ok && !update.Detached && editor.ValidateAnchor(content, update.Anchor) == nil {
			anchor = update.Anchor
			mapped = true
		}
		status = documentannotation.StatusAfterCopy(status, mapped, preserveUnmapped)
		_, err := tx.Exec(ctx, `
			INSERT INTO document_annotations(
				id,project_id,node_id,kind,dictionary_term_id,status,block_id,start_offset,
				end_offset,exact_text,prefix,suffix,note,target_snapshot,base_revision,resolved_at
			) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		`, uuid.NewString(), projectID, toNodeID, item.kind, item.termID, status,
			anchor.BlockID, anchor.Start, anchor.End, anchor.ExactText, anchor.Prefix,
			anchor.Suffix, item.note, item.snapshot, baseRevision, item.resolvedAt)
		if err != nil {
			return err
		}
	}
	return nil
}

func validateAnnotationInput(input CreateAnnotationInput) error {
	if err := documentannotation.ValidateCreate(
		input.Kind, input.DictionaryTermID != nil, input.Note, input.BaseRevision, input.Anchor,
	); err != nil {
		return ErrInvalidAnnotation
	}
	return nil
}

func validAnnotationStatus(status string) bool {
	return documentannotation.ValidStatus(status)
}

func scanAnnotation(scanner rowScanner) (domain.DocumentAnnotation, error) {
	var item domain.DocumentAnnotation
	var snapshot []byte
	var termID *string
	var canonical, kind, language *string
	var active *bool
	if err := scanner.Scan(
		&item.ID, &item.ProjectID, &item.NodeID, &item.NodeTitle, &item.Kind, &item.Status,
		&item.AnchorStatus, &item.DictionaryTermID, &item.Anchor.BlockID, &item.Anchor.Start, &item.Anchor.End,
		&item.Anchor.ExactText, &item.Anchor.Prefix, &item.Anchor.Suffix, &item.Note,
		&item.BaseRevision, &item.Revision, &item.ResolvedAt, &item.CreatedAt, &item.UpdatedAt, &snapshot,
		&termID, &canonical, &kind, &language, &active,
	); err != nil {
		return item, err
	}
	if termID != nil && canonical != nil && kind != nil && language != nil && active != nil {
		item.DictionaryTerm = &domain.AnnotationTerm{
			ID: termID, Canonical: *canonical, Kind: *kind, Language: *language, Active: *active,
		}
	} else if item.Kind == "dictionary_link" && len(snapshot) > 0 {
		var saved domain.AnnotationTerm
		if err := json.Unmarshal(snapshot, &saved); err != nil {
			return item, err
		}
		item.DictionaryTerm = &saved
	}
	return item, nil
}

func anchorString(anchor *domain.AnnotationAnchor, get func(domain.AnnotationAnchor) string) *string {
	if anchor == nil {
		return nil
	}
	value := get(*anchor)
	return &value
}

func anchorInt(anchor *domain.AnnotationAnchor, get func(domain.AnnotationAnchor) int) *int {
	if anchor == nil {
		return nil
	}
	value := get(*anchor)
	return &value
}

func mapKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	return result
}

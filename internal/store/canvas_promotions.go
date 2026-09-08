package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	canvasdocument "writers.studio/internal/canvas"
	"writers.studio/internal/domain"
	"writers.studio/internal/markdowntree"
)

type PromoteCanvasNodeInput struct {
	BaseRevision int
	NodeID       string
	Title        string
	Kind         string
	ParentID     *string
}

type CanvasPromotion struct {
	Document domain.DocumentNode   `json:"document"`
	Revision domain.CanvasRevision `json:"revision"`
}

type CanvasPromotionConflictError struct {
	CurrentRevision int
}

func (e *CanvasPromotionConflictError) Error() string {
	return fmt.Sprintf("canvas promotion base revision is stale: current=%d", e.CurrentRevision)
}

func AsCanvasPromotionConflict(err error) (*CanvasPromotionConflictError, bool) {
	var conflict *CanvasPromotionConflictError
	ok := errors.As(err, &conflict)
	return conflict, ok
}

func (s *Store) PromoteCanvasNode(
	ctx context.Context,
	ownerID, canvasID string,
	input PromoteCanvasNodeInput,
) (CanvasPromotion, error) {
	var result CanvasPromotion
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var currentRevision int
		var projectID string
		var rawCanvas, structureJSON []byte
		if err := tx.QueryRow(ctx, `
			SELECT c.current_revision, c.project_id::text, r.content, p.structure_schema
			FROM canvases c
			JOIN canvas_revisions r ON r.canvas_id=c.id AND r.revision=c.current_revision
			JOIN projects p ON p.id=c.project_id
			WHERE c.id=$1 AND p.owner_user_id=$2 AND c.trashed_at IS NULL AND p.status <> 'trash'
			FOR UPDATE OF c, p
		`, canvasID, ownerID).Scan(&currentRevision, &projectID, &rawCanvas, &structureJSON); err != nil {
			return err
		}
		if currentRevision != input.BaseRevision {
			return &CanvasPromotionConflictError{CurrentRevision: currentRevision}
		}
		var structure domain.ProjectStructure
		if err := json.Unmarshal(structureJSON, &structure); err != nil {
			return err
		}
		var parentKind *string
		if input.ParentID != nil {
			var value string
			if err := tx.QueryRow(ctx, `
				SELECT kind FROM document_nodes
				WHERE id=$1 AND project_id=$2 AND trashed_at IS NULL
				FOR UPDATE
			`, *input.ParentID, projectID).Scan(&value); err != nil {
				return err
			}
			parentKind = &value
		}
		if err := validateNodePlacement(structure, input.Kind, parentKind); err != nil {
			return err
		}
		contentBearing := false
		for _, nodeType := range structure.NodeTypes {
			if nodeType.Key == input.Kind {
				contentBearing = nodeType.ContentBearing
				break
			}
		}
		if !contentBearing {
			return ErrInvalidParentType
		}

		documentID := uuid.NewString()
		promotion, err := canvasdocument.PromoteNode(rawCanvas, input.NodeID, canvasID, documentID)
		if err != nil {
			return err
		}
		parsed, err := markdowntree.ParseMarkdown("canvas-promotion.md", []byte(promotion.SourceText), nil)
		if err != nil {
			return err
		}
		var position float64
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(MAX(position), 0)::float8 + 1000
			FROM document_nodes
			WHERE project_id=$1 AND parent_id IS NOT DISTINCT FROM $2
		`, projectID, input.ParentID).Scan(&position); err != nil {
			return err
		}
		frontmatter := fmt.Sprintf(
			"---\nwriters_studio_canvas_id: %s\nwriters_studio_canvas_node_id: %s\n---\n",
			canvasID, input.NodeID,
		)
		if err := tx.QueryRow(ctx, `
			INSERT INTO document_nodes(
				id, project_id, parent_id, kind, title, position, current_revision, source_frontmatter
			) VALUES($1, $2, $3, $4, $5, $6, 1, $7)
			RETURNING id::text, project_id::text, parent_id::text, kind, title, status,
				position::float8, current_revision, trashed_at, created_at, updated_at
		`, documentID, projectID, input.ParentID, input.Kind, input.Title, position, frontmatter).Scan(
			&result.Document.ID, &result.Document.ProjectID, &result.Document.ParentID,
			&result.Document.Kind, &result.Document.Title, &result.Document.Status,
			&result.Document.Position, &result.Document.CurrentRevision, &result.Document.TrashedAt,
			&result.Document.CreatedAt, &result.Document.UpdatedAt,
		); err != nil {
			return err
		}
		result.Document.Content = parsed.Content
		result.Document.PlainText = parsed.PlainText
		if _, err := tx.Exec(ctx, `
			INSERT INTO document_revisions(
				id, node_id, revision, base_revision, schema_version, content, plain_text, source
			) VALUES($1, $2, 1, 0, $3, $4, $5, 'canvas_promotion')
		`, uuid.NewString(), documentID, 3, parsed.Content, parsed.PlainText); err != nil {
			return err
		}

		references, err := canvasdocument.Validate(promotion.Content)
		if err != nil {
			return err
		}
		if err := validateCanvasResourcesTx(ctx, tx, ownerID, projectID, references); err != nil {
			return err
		}
		result.Revision = domain.CanvasRevision{
			ID: uuid.NewString(), CanvasID: canvasID, Revision: currentRevision + 1,
			BaseRevision: currentRevision, SchemaVersion: references.SchemaVersion,
			Content: promotion.Content, Source: "canvas_promotion", CreatedAt: time.Now(),
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO canvas_revisions(
				id, canvas_id, revision, base_revision, schema_version, content, source, created_at
			) VALUES($1, $2, $3, $4, $5, $6, $7, $8)
		`, result.Revision.ID, canvasID, result.Revision.Revision,
			result.Revision.BaseRevision, result.Revision.SchemaVersion, result.Revision.Content,
			result.Revision.Source, result.Revision.CreatedAt); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE canvases SET current_revision=$2, updated_at=now() WHERE id=$1
		`, canvasID, result.Revision.Revision)
		return err
	})
	return result, err
}

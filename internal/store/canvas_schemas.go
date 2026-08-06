package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	canvasdocument "writers.studio/internal/canvas"
)

type CanvasSchemaRevisionConflictError struct {
	CurrentRevision int
}

func (e *CanvasSchemaRevisionConflictError) Error() string {
	return fmt.Sprintf("canvas schema base revision is stale: current=%d", e.CurrentRevision)
}

func AsCanvasSchemaConflict(err error) (*CanvasSchemaRevisionConflictError, bool) {
	var conflict *CanvasSchemaRevisionConflictError
	ok := errors.As(err, &conflict)
	return conflict, ok
}

func (s *Store) GetCanvasTypeSchema(
	ctx context.Context,
	ownerID, projectID string,
) (canvasdocument.TypeSchema, error) {
	var revision int
	var raw []byte
	err := s.Pool.QueryRow(ctx, `
		SELECT COALESCE(cs.current_revision, 0), csr.schema
		FROM projects p
		LEFT JOIN canvas_schemas cs ON cs.project_id=p.id
		LEFT JOIN canvas_schema_revisions csr
			ON csr.project_id=p.id AND csr.revision=cs.current_revision
		WHERE p.id=$1 AND p.owner_user_id=$2 AND p.status <> 'trash'
	`, projectID, ownerID).Scan(&revision, &raw)
	if err != nil {
		return canvasdocument.TypeSchema{}, err
	}
	if revision == 0 {
		return canvasdocument.TypeSchema{
			Revision: 0, CardTypes: []canvasdocument.CardType{}, RelationTypes: []canvasdocument.RelationType{},
		}, nil
	}
	var result canvasdocument.TypeSchema
	if err := json.Unmarshal(raw, &result); err != nil {
		return canvasdocument.TypeSchema{}, fmt.Errorf("decode canvas schema: %w", err)
	}
	result.Revision = revision
	return result, nil
}

func (s *Store) SaveCanvasTypeSchema(
	ctx context.Context,
	ownerID, projectID string,
	baseRevision int,
	value canvasdocument.TypeSchema,
	source string,
) (canvasdocument.TypeSchema, error) {
	var result canvasdocument.TypeSchema
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var projectExists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM projects
				WHERE id=$1 AND owner_user_id=$2 AND status <> 'trash'
			)
		`, projectID, ownerID).Scan(&projectExists); err != nil {
			return err
		}
		if !projectExists {
			return pgx.ErrNoRows
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO canvas_schemas(project_id, current_revision)
			VALUES($1, 0)
			ON CONFLICT(project_id) DO NOTHING
		`, projectID); err != nil {
			return err
		}
		var currentRevision int
		if err := tx.QueryRow(ctx, `
			SELECT current_revision FROM canvas_schemas WHERE project_id=$1 FOR UPDATE
		`, projectID).Scan(&currentRevision); err != nil {
			return err
		}
		if currentRevision != baseRevision {
			return &CanvasSchemaRevisionConflictError{CurrentRevision: currentRevision}
		}
		result = value
		result.Revision = currentRevision + 1
		encoded, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO canvas_schema_revisions(
				id, project_id, revision, base_revision, schema, source
			) VALUES($1, $2, $3, $4, $5, $6)
		`, uuid.NewString(), projectID, result.Revision, currentRevision, encoded, source); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE canvas_schemas
			SET current_revision=$2, updated_at=now()
			WHERE project_id=$1
		`, projectID, result.Revision)
		return err
	})
	return result, err
}

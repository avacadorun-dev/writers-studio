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

	canvasdocument "writers.studio/internal/canvas"
	"writers.studio/internal/domain"
)

type CanvasRevisionConflictError struct {
	CurrentRevision  int
	ConflictCanvasID string
}

func (e *CanvasRevisionConflictError) Error() string {
	return fmt.Sprintf("canvas base revision is stale: current=%d", e.CurrentRevision)
}

func AsCanvasConflict(err error) (*CanvasRevisionConflictError, bool) {
	var conflict *CanvasRevisionConflictError
	ok := errors.As(err, &conflict)
	return conflict, ok
}

type UpdateCanvasInput struct {
	Title    *string
	Position *float64
}

type SaveCanvasRevisionInput struct {
	BaseRevision  int
	SchemaVersion int
	Content       json.RawMessage
	Source        string
	References    canvasdocument.References
}

func (s *Store) ListCanvases(
	ctx context.Context,
	ownerID, projectID string,
	includeTrashed bool,
) ([]domain.CanvasSummary, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT c.id::text, c.project_id::text, c.title, c.position::float8,
			c.current_revision, c.trashed_at, c.created_at, c.updated_at
		FROM canvases c
		JOIN projects p ON p.id=c.project_id
		WHERE c.project_id=$1 AND p.owner_user_id=$2 AND ($3 OR c.trashed_at IS NULL)
		ORDER BY c.position, c.created_at
	`, projectID, ownerID, includeTrashed)
	if err != nil {
		return nil, fmt.Errorf("list canvases: %w", err)
	}
	defer rows.Close()
	result := make([]domain.CanvasSummary, 0)
	for rows.Next() {
		var item domain.CanvasSummary
		if err := rows.Scan(
			&item.ID, &item.ProjectID, &item.Title, &item.Position,
			&item.CurrentRevision, &item.TrashedAt, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan canvas: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) CreateCanvas(
	ctx context.Context,
	ownerID, projectID, title string,
) (domain.CanvasSummary, error) {
	content := canvasdocument.EmptyDocument()
	var result domain.CanvasSummary
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var position float64
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(MAX(c.position), 0)::float8 + 1000
			FROM projects p
			LEFT JOIN canvases c ON c.project_id=p.id
			WHERE p.id=$1 AND p.owner_user_id=$2 AND p.status <> 'trash'
		`, projectID, ownerID).Scan(&position); err != nil {
			return err
		}
		canvasID := uuid.NewString()
		if err := tx.QueryRow(ctx, `
			INSERT INTO canvases(id, project_id, title, position, current_revision)
			SELECT $1, p.id, $3, $4, 1
			FROM projects p
			WHERE p.id=$2 AND p.owner_user_id=$5 AND p.status <> 'trash'
			RETURNING id::text, project_id::text, title, position::float8,
				current_revision, trashed_at, created_at, updated_at
		`, canvasID, projectID, title, position, ownerID).Scan(
			&result.ID, &result.ProjectID, &result.Title, &result.Position,
			&result.CurrentRevision, &result.TrashedAt, &result.CreatedAt, &result.UpdatedAt,
		); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO canvas_revisions(
				id, canvas_id, revision, base_revision, schema_version, content, source
			) VALUES($1, $2, 1, 0, $3, $4, 'canvas_create')
		`, uuid.NewString(), canvasID, canvasdocument.SchemaVersion, content)
		return err
	})
	return result, err
}

func (s *Store) GetCanvasWorkspace(
	ctx context.Context,
	ownerID, canvasID string,
) (domain.CanvasWorkspace, error) {
	var result domain.CanvasWorkspace
	err := s.Pool.QueryRow(ctx, `
		SELECT c.id::text, c.project_id::text, c.title, c.position::float8,
			c.current_revision, c.trashed_at, c.created_at, c.updated_at,
			r.id::text, r.canvas_id::text, r.revision, r.base_revision,
			r.schema_version, r.content, r.source, r.created_at
		FROM canvases c
		JOIN projects p ON p.id=c.project_id
		JOIN canvas_revisions r ON r.canvas_id=c.id AND r.revision=c.current_revision
		WHERE c.id=$1 AND p.owner_user_id=$2
	`, canvasID, ownerID).Scan(
		&result.Canvas.ID, &result.Canvas.ProjectID, &result.Canvas.Title,
		&result.Canvas.Position, &result.Canvas.CurrentRevision, &result.Canvas.TrashedAt,
		&result.Canvas.CreatedAt, &result.Canvas.UpdatedAt,
		&result.Revision.ID, &result.Revision.CanvasID, &result.Revision.Revision,
		&result.Revision.BaseRevision, &result.Revision.SchemaVersion,
		&result.Revision.Content, &result.Revision.Source, &result.Revision.CreatedAt,
	)
	if err != nil {
		return domain.CanvasWorkspace{}, err
	}
	references, err := canvasdocument.Validate(result.Revision.Content)
	if err != nil {
		return domain.CanvasWorkspace{}, fmt.Errorf("stored canvas is invalid: %w", err)
	}
	result.Documents, err = s.canvasDocumentPreviews(
		ctx, ownerID, result.Canvas.ProjectID, references.DocumentIDs,
	)
	if err != nil {
		return domain.CanvasWorkspace{}, err
	}
	result.Media, err = s.canvasMediaPreviews(
		ctx, ownerID, result.Canvas.ProjectID, references.MediaIDs,
	)
	if err != nil {
		return domain.CanvasWorkspace{}, err
	}
	return result, nil
}

func (s *Store) UpdateCanvas(
	ctx context.Context,
	ownerID, canvasID string,
	input UpdateCanvasInput,
) (domain.CanvasSummary, error) {
	var result domain.CanvasSummary
	err := s.Pool.QueryRow(ctx, `
		UPDATE canvases c
		SET title=COALESCE($3, c.title), position=COALESCE($4, c.position), updated_at=now()
		FROM projects p
		WHERE c.id=$1 AND c.project_id=p.id AND p.owner_user_id=$2
		RETURNING c.id::text, c.project_id::text, c.title, c.position::float8,
			c.current_revision, c.trashed_at, c.created_at, c.updated_at
	`, canvasID, ownerID, input.Title, input.Position).Scan(
		&result.ID, &result.ProjectID, &result.Title, &result.Position,
		&result.CurrentRevision, &result.TrashedAt, &result.CreatedAt, &result.UpdatedAt,
	)
	return result, err
}

func (s *Store) DuplicateCanvas(
	ctx context.Context,
	ownerID, canvasID string,
) (domain.CanvasSummary, error) {
	var result domain.CanvasSummary
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var projectID, title string
		var content json.RawMessage
		if err := tx.QueryRow(ctx, `
			SELECT c.project_id::text, c.title, r.content
			FROM canvases c
			JOIN projects p ON p.id=c.project_id
			JOIN canvas_revisions r ON r.canvas_id=c.id AND r.revision=c.current_revision
			WHERE c.id=$1 AND p.owner_user_id=$2 AND c.trashed_at IS NULL
			FOR UPDATE OF c
		`, canvasID, ownerID).Scan(&projectID, &title, &content); err != nil {
			return err
		}
		var position float64
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(MAX(position), 0)::float8 + 1000 FROM canvases WHERE project_id=$1
		`, projectID).Scan(&position); err != nil {
			return err
		}
		newID := uuid.NewString()
		if err := tx.QueryRow(ctx, `
			INSERT INTO canvases(id, project_id, title, position, current_revision)
			VALUES($1, $2, $3, $4, 1)
			RETURNING id::text, project_id::text, title, position::float8,
				current_revision, trashed_at, created_at, updated_at
		`, newID, projectID, canvasCopyTitle(title, " — копия"), position).Scan(
			&result.ID, &result.ProjectID, &result.Title, &result.Position,
			&result.CurrentRevision, &result.TrashedAt, &result.CreatedAt, &result.UpdatedAt,
		); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO canvas_revisions(
				id, canvas_id, revision, base_revision, schema_version, content, source
			) VALUES($1, $2, 1, 0, $3, $4, 'canvas_duplicate')
		`, uuid.NewString(), newID, canvasdocument.SchemaVersion, content)
		return err
	})
	return result, err
}

func (s *Store) TrashCanvas(ctx context.Context, ownerID, canvasID string) (domain.CanvasSummary, error) {
	now := time.Now()
	return s.setCanvasTrashedAt(ctx, ownerID, canvasID, &now)
}

func (s *Store) RestoreCanvas(ctx context.Context, ownerID, canvasID string) (domain.CanvasSummary, error) {
	return s.setCanvasTrashedAt(ctx, ownerID, canvasID, nil)
}

func (s *Store) setCanvasTrashedAt(
	ctx context.Context,
	ownerID, canvasID string,
	trashedAt *time.Time,
) (domain.CanvasSummary, error) {
	var result domain.CanvasSummary
	err := s.Pool.QueryRow(ctx, `
		UPDATE canvases c
		SET trashed_at=$3, updated_at=now()
		FROM projects p
		WHERE c.id=$1 AND c.project_id=p.id AND p.owner_user_id=$2
		RETURNING c.id::text, c.project_id::text, c.title, c.position::float8,
			c.current_revision, c.trashed_at, c.created_at, c.updated_at
	`, canvasID, ownerID, trashedAt).Scan(
		&result.ID, &result.ProjectID, &result.Title, &result.Position,
		&result.CurrentRevision, &result.TrashedAt, &result.CreatedAt, &result.UpdatedAt,
	)
	return result, err
}

func (s *Store) SaveCanvasRevision(
	ctx context.Context,
	ownerID, canvasID string,
	input SaveCanvasRevisionInput,
) (domain.CanvasRevision, error) {
	var result domain.CanvasRevision
	var savedConflict *CanvasRevisionConflictError
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var current int
		var projectID, title string
		var position float64
		if err := tx.QueryRow(ctx, `
			SELECT c.current_revision, c.project_id::text, c.title, c.position::float8
			FROM canvases c
			JOIN projects p ON p.id=c.project_id
			WHERE c.id=$1 AND p.owner_user_id=$2 AND c.trashed_at IS NULL
			FOR UPDATE OF c
		`, canvasID, ownerID).Scan(&current, &projectID, &title, &position); err != nil {
			return err
		}
		if err := validateCanvasResourcesTx(
			ctx, tx, ownerID, projectID, input.References,
		); err != nil {
			return err
		}
		if current != input.BaseRevision {
			conflictID := uuid.NewString()
			var nextPosition float64
			if err := tx.QueryRow(ctx, `
				SELECT COALESCE(MAX(position), $1)::float8 + 1 FROM canvases WHERE project_id=$2
			`, position, projectID).Scan(&nextPosition); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO canvases(id, project_id, title, position, current_revision)
				VALUES($1, $2, $3, $4, 1)
			`, conflictID, projectID, canvasCopyTitle(title, " — конфликтующая копия"), nextPosition); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO canvas_revisions(
					id, canvas_id, revision, base_revision, schema_version, content, source
				) VALUES($1, $2, 1, 0, $3, $4, 'conflict_copy')
			`, uuid.NewString(), conflictID, input.SchemaVersion, input.Content); err != nil {
				return err
			}
			savedConflict = &CanvasRevisionConflictError{
				CurrentRevision: current, ConflictCanvasID: conflictID,
			}
			return nil
		}

		result = domain.CanvasRevision{
			ID: uuid.NewString(), CanvasID: canvasID, Revision: current + 1,
			BaseRevision: current, SchemaVersion: input.SchemaVersion,
			Content: input.Content, Source: input.Source, CreatedAt: time.Now(),
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO canvas_revisions(
				id, canvas_id, revision, base_revision, schema_version, content, source, created_at
			) VALUES($1, $2, $3, $4, $5, $6, $7, $8)
		`, result.ID, canvasID, result.Revision, result.BaseRevision,
			result.SchemaVersion, result.Content, result.Source, result.CreatedAt); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			UPDATE canvases SET current_revision=$2, updated_at=now() WHERE id=$1
		`, canvasID, result.Revision)
		return err
	})
	if err != nil {
		return result, err
	}
	if savedConflict != nil {
		return result, savedConflict
	}
	return result, nil
}

func validateCanvasResourcesTx(
	ctx context.Context,
	tx pgx.Tx,
	ownerID, projectID string,
	references canvasdocument.References,
) error {
	if len(references.DocumentIDs) > 0 {
		var count int
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM document_nodes n
			JOIN projects p ON p.id=n.project_id
			WHERE n.project_id=$1 AND p.owner_user_id=$2 AND n.id::text=ANY($3::text[])
		`, projectID, ownerID, references.DocumentIDs).Scan(&count); err != nil {
			return err
		}
		if count != len(references.DocumentIDs) {
			return fmt.Errorf("%w: document resource is outside the project", canvasdocument.ErrInvalidDocument)
		}
	}
	if len(references.MediaIDs) > 0 {
		var count int
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM project_media m
			JOIN projects p ON p.id=m.project_id
			WHERE m.project_id=$1 AND p.owner_user_id=$2 AND m.id::text=ANY($3::text[])
		`, projectID, ownerID, references.MediaIDs).Scan(&count); err != nil {
			return err
		}
		if count != len(references.MediaIDs) {
			return fmt.Errorf("%w: media resource is outside the project", canvasdocument.ErrInvalidDocument)
		}
	}
	return nil
}

func (s *Store) canvasDocumentPreviews(
	ctx context.Context,
	ownerID, projectID string,
	ids []string,
) ([]domain.CanvasDocumentPreview, error) {
	result := make([]domain.CanvasDocumentPreview, 0, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT n.id::text, n.title, n.kind, n.status, n.current_revision,
			LEFT(COALESCE(r.plain_text, ''), 1200), n.trashed_at
		FROM document_nodes n
		JOIN projects p ON p.id=n.project_id
		LEFT JOIN document_revisions r ON r.node_id=n.id AND r.revision=n.current_revision
		WHERE n.project_id=$1 AND p.owner_user_id=$2 AND n.id::text=ANY($3::text[])
	`, projectID, ownerID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item domain.CanvasDocumentPreview
		if err := rows.Scan(
			&item.ID, &item.Title, &item.Kind, &item.Status, &item.CurrentRevision,
			&item.PlainText, &item.TrashedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) canvasMediaPreviews(
	ctx context.Context,
	ownerID, projectID string,
	ids []string,
) ([]domain.ProjectMedia, error) {
	result := make([]domain.ProjectMedia, 0, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT m.id::text, m.project_id::text, m.media_type, m.original_name, m.sha256,
			m.byte_size, m.width, m.height, m.source_path, m.storage_key, m.created_at
		FROM project_media m
		JOIN projects p ON p.id=m.project_id
		WHERE m.project_id=$1 AND p.owner_user_id=$2 AND m.id::text=ANY($3::text[])
	`, projectID, ownerID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item domain.ProjectMedia
		if err := rows.Scan(
			&item.ID, &item.ProjectID, &item.MediaType, &item.OriginalName, &item.SHA256,
			&item.ByteSize, &item.Width, &item.Height, &item.SourcePath, &item.StorageKey,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		item.ContentURL = "/api/media/" + item.ID + "/content"
		result = append(result, item)
	}
	return result, rows.Err()
}

func canvasCopyTitle(title, suffix string) string {
	maximum := 160 - len([]rune(suffix))
	value := []rune(strings.TrimSpace(title))
	if len(value) > maximum {
		value = value[:maximum]
	}
	return string(value) + suffix
}

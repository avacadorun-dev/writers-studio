package store

import (
	"context"

	"github.com/google/uuid"

	"writers.studio/internal/domain"
)

type CreateProjectMediaInput struct {
	ProjectID    string
	MediaType    string
	OriginalName string
	SourcePath   *string
	SHA256       string
	ByteSize     int64
	Width        *int
	Height       *int
	PageCount    *int
	DurationMS   *int64
	StorageKey   string
}

func (s *Store) CreateProjectMedia(
	ctx context.Context,
	ownerID string,
	input CreateProjectMediaInput,
) (domain.ProjectMedia, error) {
	var media domain.ProjectMedia
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO project_media(
			id, project_id, owner_user_id, storage_key, media_type, original_name,
			sha256, byte_size, width, height, page_count, duration_ms, source_path
		)
		SELECT $1, p.id, p.owner_user_id, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		FROM projects p
		WHERE p.id=$2 AND p.owner_user_id=$3 AND p.status <> 'trash'
		RETURNING id::text, project_id::text, media_type, original_name, sha256,
			byte_size, width, height, page_count, duration_ms, source_path, storage_key, created_at
	`, uuid.NewString(), input.ProjectID, ownerID, input.StorageKey, input.MediaType,
		input.OriginalName, input.SHA256, input.ByteSize, input.Width, input.Height,
		input.PageCount, input.DurationMS, input.SourcePath).Scan(
		&media.ID,
		&media.ProjectID,
		&media.MediaType,
		&media.OriginalName,
		&media.SHA256,
		&media.ByteSize,
		&media.Width,
		&media.Height,
		&media.PageCount,
		&media.DurationMS,
		&media.SourcePath,
		&media.StorageKey,
		&media.CreatedAt,
	)
	if err != nil {
		return domain.ProjectMedia{}, err
	}
	media.ContentURL = "/api/media/" + media.ID + "/content"
	return media, nil
}

func (s *Store) GetProjectMedia(
	ctx context.Context,
	ownerID, mediaID string,
) (domain.ProjectMedia, error) {
	var media domain.ProjectMedia
	err := s.Pool.QueryRow(ctx, `
		SELECT m.id::text, m.project_id::text, m.media_type, m.original_name, m.sha256,
			m.byte_size, m.width, m.height, m.page_count, m.duration_ms,
			m.source_path, m.storage_key, m.created_at
		FROM project_media m
		JOIN projects p ON p.id=m.project_id
		WHERE m.id=$1 AND p.owner_user_id=$2
	`, mediaID, ownerID).Scan(
		&media.ID,
		&media.ProjectID,
		&media.MediaType,
		&media.OriginalName,
		&media.SHA256,
		&media.ByteSize,
		&media.Width,
		&media.Height,
		&media.PageCount,
		&media.DurationMS,
		&media.SourcePath,
		&media.StorageKey,
		&media.CreatedAt,
	)
	if err != nil {
		return domain.ProjectMedia{}, err
	}
	media.ContentURL = "/api/media/" + media.ID + "/content"
	return media, nil
}

func (s *Store) ListProjectMedia(
	ctx context.Context,
	ownerID, projectID string,
) ([]domain.ProjectMedia, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT m.id::text, m.project_id::text, m.media_type, m.original_name, m.sha256,
			m.byte_size, m.width, m.height, m.page_count, m.duration_ms,
			m.source_path, m.storage_key, m.created_at
		FROM project_media m
		JOIN projects p ON p.id=m.project_id
		WHERE m.project_id=$1 AND p.owner_user_id=$2
		ORDER BY m.created_at DESC
	`, projectID, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.ProjectMedia, 0)
	for rows.Next() {
		var media domain.ProjectMedia
		if err := rows.Scan(&media.ID, &media.ProjectID, &media.MediaType, &media.OriginalName,
			&media.SHA256, &media.ByteSize, &media.Width, &media.Height,
			&media.PageCount, &media.DurationMS, &media.SourcePath, &media.StorageKey,
			&media.CreatedAt); err != nil {
			return nil, err
		}
		media.ContentURL = "/api/media/" + media.ID + "/content"
		result = append(result, media)
	}
	return result, rows.Err()
}

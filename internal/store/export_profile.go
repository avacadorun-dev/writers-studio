package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"writers.studio/internal/domain"
	exportformat "writers.studio/internal/export"
)

var ErrExportProfileStale = errors.New("export profile revision is stale")

func (s *Store) GetExportProfile(
	ctx context.Context,
	ownerID, projectID string,
) (domain.ExportProfile, int, error) {
	var encoded []byte
	var revision int
	var title, synopsis string
	err := s.Pool.QueryRow(ctx, `
		SELECT export_profile, export_profile_revision, title, synopsis
		FROM projects WHERE id=$1 AND owner_user_id=$2
	`, projectID, ownerID).Scan(&encoded, &revision, &title, &synopsis)
	if err != nil {
		return domain.ExportProfile{}, 0, err
	}
	var profile domain.ExportProfile
	if err := json.Unmarshal(encoded, &profile); err != nil {
		return domain.ExportProfile{}, 0, err
	}
	profile = exportformat.NormalizeProfile(profile, title, synopsis)
	if err := exportformat.ValidateProfile(profile); err != nil {
		return domain.ExportProfile{}, 0, err
	}
	return profile, revision, nil
}

func (s *Store) SaveExportProfile(
	ctx context.Context,
	ownerID, projectID string,
	baseRevision int,
	profile domain.ExportProfile,
) (domain.ExportProfile, int, error) {
	var nextRevision int
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var currentRevision int
		var title, synopsis string
		if err := tx.QueryRow(ctx, `
			SELECT export_profile_revision, title, synopsis
			FROM projects WHERE id=$1 AND owner_user_id=$2 FOR UPDATE
		`, projectID, ownerID).Scan(&currentRevision, &title, &synopsis); err != nil {
			return err
		}
		if currentRevision != baseRevision {
			return ErrExportProfileStale
		}
		profile = exportformat.NormalizeProfile(profile, title, synopsis)
		if err := exportformat.ValidateProfile(profile); err != nil {
			return err
		}
		if profile.Metadata.CoverAssetID != nil {
			var exists bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS(SELECT 1 FROM project_media WHERE id=$1 AND project_id=$2 AND owner_user_id=$3)
			`, *profile.Metadata.CoverAssetID, projectID, ownerID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return exportformat.ErrInvalidProfile
			}
		}
		encoded, err := json.Marshal(profile)
		if err != nil {
			return err
		}
		nextRevision = currentRevision + 1
		_, err = tx.Exec(ctx, `
			UPDATE projects SET export_profile=$3, export_profile_revision=$4, updated_at=now()
			WHERE id=$1 AND owner_user_id=$2
		`, projectID, ownerID, encoded, nextRevision)
		return err
	})
	return profile, nextRevision, err
}

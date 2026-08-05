package store

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type UserPreferences struct {
	AudioRetentionDays int `json:"audioRetentionDays"`
}

type BackupAudio struct {
	SessionID  string
	StorageKey string
}

type BackupMedia struct {
	ID         string
	StorageKey string
	MediaType  string
}

type AccountPurgePlan struct {
	Email      string
	ObjectKeys []string
}

func (s *Store) GetUserPreferences(
	ctx context.Context,
	userID string,
) (UserPreferences, error) {
	var preferences UserPreferences
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO user_preferences(user_id)
		VALUES($1)
		ON CONFLICT(user_id) DO UPDATE SET user_id=EXCLUDED.user_id
		RETURNING audio_retention_days
	`, userID).Scan(&preferences.AudioRetentionDays)
	return preferences, err
}

func (s *Store) UpdateUserPreferences(
	ctx context.Context,
	userID string,
	audioRetentionDays int,
) (UserPreferences, error) {
	var preferences UserPreferences
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO user_preferences(user_id, audio_retention_days)
		VALUES($1, $2)
		ON CONFLICT(user_id) DO UPDATE
		SET audio_retention_days=EXCLUDED.audio_retention_days, updated_at=now()
		RETURNING audio_retention_days
	`, userID, audioRetentionDays).Scan(&preferences.AudioRetentionDays)
	return preferences, err
}

func (s *Store) ListExpiredAudioKeys(
	ctx context.Context,
	userID string,
	retentionDays int,
) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT chunk.storage_key
		FROM audio_chunks chunk
		JOIN dictation_sessions session ON session.id=chunk.session_id
		WHERE session.owner_user_id=$1
		  AND session.created_at < now() - make_interval(days => $2)
		UNION
		SELECT audio.storage_key
		FROM audio_assets audio
		JOIN dictation_sessions session ON session.id=audio.session_id
		WHERE session.owner_user_id=$1
		  AND session.created_at < now() - make_interval(days => $2)
	`, userID, retentionDays)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := make([]string, 0)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *Store) DeleteExpiredAudio(
	ctx context.Context,
	userID string,
	retentionDays int,
) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			DELETE FROM audio_chunks chunk
			USING dictation_sessions session
			WHERE chunk.session_id=session.id
			  AND session.owner_user_id=$1
			  AND session.created_at < now() - make_interval(days => $2)
		`, userID, retentionDays); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			DELETE FROM audio_assets audio
			USING dictation_sessions session
			WHERE audio.session_id=session.id
			  AND session.owner_user_id=$1
			  AND session.created_at < now() - make_interval(days => $2)
		`, userID, retentionDays)
		return err
	})
}

func (s *Store) BuildAccountBackup(
	ctx context.Context,
	userID string,
) (json.RawMessage, []BackupAudio, []BackupMedia, error) {
	var data json.RawMessage
	err := s.Pool.QueryRow(ctx, `
		SELECT jsonb_build_object(
			'formatVersion', 2,
			'exportedAt', now(),
			'projects', (
				SELECT COALESCE(jsonb_agg(to_jsonb(project) - 'owner_user_id'
					ORDER BY project.created_at), '[]'::jsonb)
				FROM projects project WHERE project.owner_user_id=$1
			),
			'documentNodes', (
				SELECT COALESCE(jsonb_agg(to_jsonb(node) ORDER BY node.created_at), '[]'::jsonb)
				FROM document_nodes node
				JOIN projects project ON project.id=node.project_id
				WHERE project.owner_user_id=$1
			),
			'documentRevisions', (
				SELECT COALESCE(jsonb_agg(to_jsonb(revision) ORDER BY revision.created_at), '[]'::jsonb)
				FROM document_revisions revision
				JOIN document_nodes node ON node.id=revision.node_id
				JOIN projects project ON project.id=node.project_id
				WHERE project.owner_user_id=$1
			),
			'namedVersions', (
				SELECT COALESCE(jsonb_agg(to_jsonb(version) ORDER BY version.created_at), '[]'::jsonb)
				FROM named_versions version
				JOIN document_nodes node ON node.id=version.node_id
				JOIN projects project ON project.id=node.project_id
				WHERE project.owner_user_id=$1
			),
			'dictionaryTerms', (
				SELECT COALESCE(jsonb_agg(to_jsonb(term) ORDER BY term.created_at), '[]'::jsonb)
				FROM dictionary_terms term
				JOIN projects project ON project.id=term.project_id
				WHERE project.owner_user_id=$1
			),
			'projectMedia', (
				SELECT COALESCE(jsonb_agg(to_jsonb(media) - 'owner_user_id' - 'storage_key'
					ORDER BY media.created_at), '[]'::jsonb)
				FROM project_media media WHERE media.owner_user_id=$1
			),
			'dictationSessions', (
				SELECT COALESCE(jsonb_agg(to_jsonb(session) - 'owner_user_id'
					ORDER BY session.created_at), '[]'::jsonb)
				FROM dictation_sessions session WHERE session.owner_user_id=$1
			),
			'transcriptArtifacts', (
				SELECT COALESCE(jsonb_agg(to_jsonb(artifact) ORDER BY artifact.created_at), '[]'::jsonb)
				FROM transcript_artifacts artifact
				JOIN dictation_sessions session ON session.id=artifact.session_id
				WHERE session.owner_user_id=$1
			),
			'transcriptDrafts', (
				SELECT COALESCE(jsonb_agg(to_jsonb(draft) ORDER BY draft.created_at), '[]'::jsonb)
				FROM transcript_drafts draft
				JOIN dictation_sessions session ON session.id=draft.session_id
				WHERE session.owner_user_id=$1
			),
			'transcriptDraftRevisions', (
				SELECT COALESCE(jsonb_agg(to_jsonb(revision) ORDER BY revision.created_at), '[]'::jsonb)
				FROM transcript_draft_revisions revision
				JOIN dictation_sessions session ON session.id=revision.session_id
				WHERE session.owner_user_id=$1
			)
		)
	`, userID).Scan(&data)
	if err != nil {
		return nil, nil, nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT audio.session_id::text, audio.storage_key
		FROM audio_assets audio
		JOIN dictation_sessions session ON session.id=audio.session_id
		WHERE session.owner_user_id=$1
		ORDER BY session.created_at
	`, userID)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	audio := make([]BackupAudio, 0)
	for rows.Next() {
		var item BackupAudio
		if err := rows.Scan(&item.SessionID, &item.StorageKey); err != nil {
			return nil, nil, nil, err
		}
		audio = append(audio, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, err
	}
	mediaRows, err := s.Pool.Query(ctx, `
		SELECT id::text, storage_key, media_type
		FROM project_media
		WHERE owner_user_id=$1
		ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, nil, nil, err
	}
	defer mediaRows.Close()
	media := make([]BackupMedia, 0)
	for mediaRows.Next() {
		var item BackupMedia
		if err := mediaRows.Scan(&item.ID, &item.StorageKey, &item.MediaType); err != nil {
			return nil, nil, nil, err
		}
		media = append(media, item)
	}
	return data, audio, media, mediaRows.Err()
}

func (s *Store) GetAccountPurgePlan(
	ctx context.Context,
	userID string,
) (AccountPurgePlan, error) {
	var plan AccountPurgePlan
	if err := s.Pool.QueryRow(ctx, `
		SELECT email
		FROM users
		WHERE id=$1
	`, userID).Scan(&plan.Email); err != nil {
		return AccountPurgePlan{}, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT chunk.storage_key
		FROM audio_chunks chunk
		JOIN dictation_sessions session ON session.id=chunk.session_id
		WHERE session.owner_user_id=$1
		UNION
		SELECT audio.storage_key
		FROM audio_assets audio
		JOIN dictation_sessions session ON session.id=audio.session_id
		WHERE session.owner_user_id=$1
		UNION
		SELECT export.storage_key
		FROM export_jobs export
		WHERE export.owner_user_id=$1 AND export.storage_key IS NOT NULL
		UNION
		SELECT media.storage_key
		FROM project_media media
		WHERE media.owner_user_id=$1
	`, userID)
	if err != nil {
		return AccountPurgePlan{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return AccountPurgePlan{}, err
		}
		plan.ObjectKeys = append(plan.ObjectKeys, key)
	}
	return plan, rows.Err()
}

func (s *Store) PurgeAccount(ctx context.Context, userID string, objectCount int) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(
			ctx,
			"SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)",
			userID,
		).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return pgx.ErrNoRows
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM background_jobs job
			WHERE job.payload->>'sessionId' IN (
				SELECT session.id::text
				FROM dictation_sessions session
				WHERE session.owner_user_id=$1
			)
			   OR job.payload->>'exportId' IN (
				SELECT export.id::text
				FROM export_jobs export
				WHERE export.owner_user_id=$1
			)
		`, userID); err != nil {
			return err
		}
		if _, err := tx.Exec(
			ctx,
			"DELETE FROM deletion_audit_events WHERE owner_user_id=$1",
			userID,
		); err != nil {
			return err
		}
		metadata, _ := json.Marshal(map[string]int{"deletedObjectCount": objectCount})
		if _, err := tx.Exec(ctx, `
			INSERT INTO deletion_audit_events(
				id, owner_user_id, subject_type, subject_id, event_type, metadata
			)
			VALUES($1, $2, 'account', $2, 'account_purged', $3)
		`, uuid.NewString(), userID, metadata); err != nil {
			return err
		}
		command, err := tx.Exec(ctx, "DELETE FROM users WHERE id=$1", userID)
		if err != nil {
			return err
		}
		if command.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

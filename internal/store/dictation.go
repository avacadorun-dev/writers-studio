package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"writers.studio/internal/domain"
)

type AudioChunk struct {
	SessionID  string
	Sequence   int
	SHA256     string
	DurationMS int
	StorageKey string
	ByteSize   int64
}

type BackgroundJob struct {
	ID             string
	Kind           string
	Status         string
	IdempotencyKey string
	Payload        json.RawMessage
	Attempts       int
}

type AudioAsset struct {
	ID         string
	SessionID  string
	SHA256     string
	StorageKey string
	ByteSize   int64
	DurationMS int64
}

type SegmentAudioSource struct {
	SessionID string
	StartMS   int64
	EndMS     int64
}

func (s *Store) CreateDictationSession(
	ctx context.Context,
	ownerID, projectID string,
	destinationNodeID *string,
	clientSessionID, language string,
) (domain.DictationSession, error) {
	var session domain.DictationSession
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO dictation_sessions(
			id, owner_user_id, project_id, destination_node_id, client_session_id, language, status
		)
		SELECT $1, p.owner_user_id, p.id, $4, $5, $6, 'recording_online'
		FROM projects p
		WHERE p.id=$2 AND p.owner_user_id=$3
		ON CONFLICT(owner_user_id, client_session_id)
		DO UPDATE SET updated_at=dictation_sessions.updated_at
		RETURNING id::text, project_id::text, destination_node_id::text, client_session_id,
			language, status, duration_ms, recorded_duration_ms, manifest, created_at, updated_at
	`, uuid.NewString(), projectID, ownerID, destinationNodeID, clientSessionID, language).Scan(
		&session.ID,
		&session.ProjectID,
		&session.DestinationNodeID,
		&session.ClientSessionID,
		&session.Language,
		&session.Status,
		&session.DurationMS,
		&session.RecordedDurationMS,
		&session.Manifest,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	return session, err
}

func (s *Store) GetDictationSession(
	ctx context.Context,
	ownerID, sessionID string,
) (domain.DictationSession, error) {
	var session domain.DictationSession
	err := s.Pool.QueryRow(ctx, `
		SELECT id::text, project_id::text, destination_node_id::text, client_session_id,
			language, status, duration_ms, recorded_duration_ms, manifest, created_at, updated_at
		FROM dictation_sessions
		WHERE id=$1 AND owner_user_id=$2
	`, sessionID, ownerID).Scan(
		&session.ID,
		&session.ProjectID,
		&session.DestinationNodeID,
		&session.ClientSessionID,
		&session.Language,
		&session.Status,
		&session.DurationMS,
		&session.RecordedDurationMS,
		&session.Manifest,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if err != nil {
		return session, err
	}

	artifacts, err := s.ListTranscriptArtifacts(ctx, ownerID, sessionID)
	if err != nil {
		return session, err
	}
	for i := range artifacts {
		switch artifacts[i].Kind {
		case "live_final":
			session.LiveArtifact = &artifacts[i]
		case "batch_source":
			session.BatchArtifact = &artifacts[i]
		case "cleaned_suggestion":
			session.SuggestionArtifact = &artifacts[i]
		}
	}
	session.RetranscriptionStatus, err = s.GetLatestRetranscriptionStatus(
		ctx,
		ownerID,
		sessionID,
	)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return session, err
	}
	draft, err := s.GetTranscriptDraft(ctx, ownerID, sessionID)
	if err == nil {
		session.Draft = &draft
		session.DraftRevisions, err = s.ListTranscriptDraftRevisions(ctx, ownerID, sessionID)
		if err != nil {
			return session, err
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return session, err
	}
	return session, nil
}

func (s *Store) TransitionDictation(
	ctx context.Context,
	ownerID, sessionID, nextStatus string,
) (domain.DictationSession, error) {
	var currentStatus string
	err := s.Pool.QueryRow(ctx, `
		SELECT status
		FROM dictation_sessions
		WHERE id=$1 AND owner_user_id=$2
	`, sessionID, ownerID).Scan(&currentStatus)
	if err != nil {
		return domain.DictationSession{}, err
	}
	if !validDictationTransition(currentStatus, nextStatus) {
		return domain.DictationSession{}, fmt.Errorf("invalid dictation transition %s -> %s", currentStatus, nextStatus)
	}
	_, err = s.Pool.Exec(ctx, `
		UPDATE dictation_sessions SET status=$3, updated_at=now()
		WHERE id=$1 AND owner_user_id=$2
	`, sessionID, ownerID, nextStatus)
	if err != nil {
		return domain.DictationSession{}, err
	}
	return s.GetDictationSession(ctx, ownerID, sessionID)
}

func (s *Store) PutAudioChunk(ctx context.Context, ownerID string, chunk AudioChunk) (bool, error) {
	var existingSHA string
	err := s.Pool.QueryRow(ctx, `
		SELECT c.sha256
		FROM audio_chunks c
		JOIN dictation_sessions s ON s.id=c.session_id
		WHERE c.session_id=$1 AND c.sequence=$2 AND s.owner_user_id=$3
	`, chunk.SessionID, chunk.Sequence, ownerID).Scan(&existingSHA)
	if err == nil {
		if existingSHA != chunk.SHA256 {
			return false, fmt.Errorf("checksum conflict for sequence %d", chunk.Sequence)
		}
		return true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}

	tag, err := s.Pool.Exec(ctx, `
		INSERT INTO audio_chunks(session_id, sequence, sha256, duration_ms, storage_key, byte_size)
		SELECT s.id, $2, $3, $4, $5, $6
		FROM dictation_sessions s
		WHERE s.id=$1 AND s.owner_user_id=$7
	`, chunk.SessionID, chunk.Sequence, chunk.SHA256, chunk.DurationMS, chunk.StorageKey, chunk.ByteSize, ownerID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, pgx.ErrNoRows
	}
	return false, nil
}

func (s *Store) ListAudioChunks(ctx context.Context, sessionID string) ([]AudioChunk, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT session_id::text, sequence, sha256, duration_ms, storage_key, byte_size
		FROM audio_chunks
		WHERE session_id=$1
		ORDER BY sequence
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	chunks := make([]AudioChunk, 0)
	for rows.Next() {
		var chunk AudioChunk
		if err := rows.Scan(
			&chunk.SessionID,
			&chunk.Sequence,
			&chunk.SHA256,
			&chunk.DurationMS,
			&chunk.StorageKey,
			&chunk.ByteSize,
		); err != nil {
			return nil, err
		}
		chunks = append(chunks, chunk)
	}
	return chunks, rows.Err()
}

func (s *Store) StopDictation(
	ctx context.Context,
	ownerID, sessionID string,
	durationMS, recordedDurationMS int64,
	manifest json.RawMessage,
) (domain.DictationSession, error) {
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `
			SELECT status
			FROM dictation_sessions
			WHERE id=$1 AND owner_user_id=$2
			FOR UPDATE
		`, sessionID, ownerID).Scan(&status)
		if err != nil {
			return err
		}
		switch status {
		case "finalizing", "review_ready", "finalization_failed", "missing_chunks":
			return nil
		case "recording_online", "recording_degraded", "paused", "stopping":
		default:
			return fmt.Errorf("cannot stop dictation in status %s", status)
		}
		_, err = tx.Exec(ctx, `
			UPDATE dictation_sessions
			SET status='finalizing', duration_ms=$3, recorded_duration_ms=$4,
				manifest=$5, stopped_at=COALESCE(stopped_at, now()), updated_at=now()
			WHERE id=$1 AND owner_user_id=$2
		`, sessionID, ownerID, durationMS, recordedDurationMS, manifest)
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]string{"sessionId": sessionID})
		_, err = tx.Exec(ctx, `
			INSERT INTO background_jobs(id, kind, status, idempotency_key, payload)
			VALUES($1, 'finalize_dictation', 'queued', $2, $3)
			ON CONFLICT(idempotency_key) DO NOTHING
		`, uuid.NewString(), "finalize:"+sessionID, payload)
		return err
	})
	if err != nil {
		return domain.DictationSession{}, err
	}
	return s.GetDictationSession(ctx, ownerID, sessionID)
}

func (s *Store) AppendLiveSegment(
	ctx context.Context,
	ownerID, sessionID, segmentID, providerTurnID, text string,
	startMS, endMS int64,
	words []domain.TranscriptWord,
	command *domain.CommandEvent,
) error {
	wordsJSON, _ := json.Marshal(words)
	var commandJSON []byte
	if command != nil {
		commandJSON, _ = json.Marshal(command)
	}
	tag, err := s.Pool.Exec(ctx, `
		INSERT INTO live_transcript_segments(
			session_id, segment_id, provider_turn_id, text, start_ms, end_ms, words, command
		)
		SELECT s.id, $3, $4, $5, $6, $7, $8, $9
		FROM dictation_sessions s
		WHERE s.id=$1 AND s.owner_user_id=$2
		ON CONFLICT(session_id, segment_id) DO NOTHING
	`, sessionID, ownerID, segmentID, providerTurnID, text, startMS, endMS, wordsJSON, commandJSON)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := s.Pool.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM live_transcript_segments
				WHERE session_id=$1 AND segment_id=$2
			)
		`, sessionID, segmentID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return pgx.ErrNoRows
		}
	}
	return nil
}

func (s *Store) MarkLiveTranscriptionStarted(
	ctx context.Context,
	ownerID, sessionID string,
) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE dictation_sessions
		SET live_finalized_at=NULL, updated_at=now()
		WHERE id=$1 AND owner_user_id=$2
	`, sessionID, ownerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) ConfigureLiveCommands(
	ctx context.Context,
	ownerID, sessionID string,
	enabled bool,
	prefix string,
) error {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "Команда"
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE dictation_sessions
		SET commands_enabled=$3, command_prefix=$4, updated_at=now()
		WHERE id=$1 AND owner_user_id=$2
	`, sessionID, ownerID, enabled, prefix)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) MarkLiveTranscriptionFinalized(
	ctx context.Context,
	ownerID, sessionID string,
) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE dictation_sessions
		SET live_finalized_at=now(), updated_at=now()
		WHERE id=$1 AND owner_user_id=$2
	`, sessionID, ownerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) SetLiveCommandExecution(
	ctx context.Context,
	ownerID, sessionID, segmentID string,
	executed bool,
) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE live_transcript_segments segment
		SET command=jsonb_set(segment.command, '{executed}', to_jsonb($4::boolean), true)
		FROM dictation_sessions session
		WHERE segment.session_id=session.id
		  AND session.id=$1 AND session.owner_user_id=$2
		  AND segment.segment_id=$3 AND segment.command IS NOT NULL
	`, sessionID, ownerID, segmentID, executed)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) UndoLastLiveCommand(
	ctx context.Context,
	ownerID, sessionID string,
) (string, error) {
	var segmentID string
	err := s.Pool.QueryRow(ctx, `
		WITH latest AS (
			SELECT segment.segment_id
			FROM live_transcript_segments segment
			JOIN dictation_sessions session ON session.id=segment.session_id
			WHERE session.id=$1 AND session.owner_user_id=$2
			  AND (segment.command->>'executed')::boolean
			  AND segment.command->>'kind' <> 'undo_command'
			ORDER BY segment.created_at DESC
			LIMIT 1
		)
		UPDATE live_transcript_segments segment
		SET command=jsonb_set(segment.command, '{executed}', 'false'::jsonb, true)
		FROM latest
		WHERE segment.session_id=$1 AND segment.segment_id=latest.segment_id
		RETURNING segment.segment_id
	`, sessionID, ownerID).Scan(&segmentID)
	return segmentID, err
}

func (s *Store) EditLiveSegment(
	ctx context.Context,
	ownerID, sessionID, segmentID, text string,
) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE live_transcript_segments segment
		SET text=$4
		FROM dictation_sessions session
		WHERE segment.session_id=session.id
		  AND session.id=$1 AND session.owner_user_id=$2
		  AND segment.segment_id=$3
	`, sessionID, ownerID, segmentID, text)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) ReplaceLiveSegments(
	ctx context.Context,
	ownerID, sessionID string,
	segmentIDs []string,
	text string,
) error {
	if len(segmentIDs) == 0 || len(segmentIDs) > 256 || len(text) > 100_000 {
		return fmt.Errorf("invalid live segment replacement")
	}
	uniqueIDs := make([]string, 0, len(segmentIDs))
	seen := make(map[string]struct{}, len(segmentIDs))
	for _, segmentID := range segmentIDs {
		segmentID = strings.TrimSpace(segmentID)
		if segmentID == "" {
			return fmt.Errorf("invalid live segment replacement")
		}
		if _, exists := seen[segmentID]; exists {
			continue
		}
		seen[segmentID] = struct{}{}
		uniqueIDs = append(uniqueIDs, segmentID)
	}
	if len(uniqueIDs) == 0 {
		return fmt.Errorf("invalid live segment replacement")
	}

	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT segment.segment_id, segment.start_ms, segment.end_ms
			FROM live_transcript_segments segment
			JOIN dictation_sessions session ON session.id=segment.session_id
			WHERE session.id=$1 AND session.owner_user_id=$2
			  AND segment.segment_id=ANY($3::text[])
			  AND (segment.command IS NULL OR segment.command='null'::jsonb)
			FOR UPDATE OF segment
		`, sessionID, ownerID, uniqueIDs)
		if err != nil {
			return err
		}

		found := make(map[string]struct{}, len(uniqueIDs))
		var startMS, endMS int64
		for rows.Next() {
			var segmentID string
			var segmentStartMS, segmentEndMS int64
			if err := rows.Scan(&segmentID, &segmentStartMS, &segmentEndMS); err != nil {
				rows.Close()
				return err
			}
			found[segmentID] = struct{}{}
			if len(found) == 1 || segmentStartMS < startMS {
				startMS = segmentStartMS
			}
			if segmentEndMS > endMS {
				endMS = segmentEndMS
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(found) != len(uniqueIDs) {
			return pgx.ErrNoRows
		}

		if strings.TrimSpace(text) == "" {
			tag, err := tx.Exec(ctx, `
				DELETE FROM live_transcript_segments
				WHERE session_id=$1 AND segment_id=ANY($2::text[])
			`, sessionID, uniqueIDs)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != int64(len(uniqueIDs)) {
				return pgx.ErrNoRows
			}
			return nil
		}

		tag, err := tx.Exec(ctx, `
			UPDATE live_transcript_segments
			SET text=$3, start_ms=$4, end_ms=$5, words='[]'::jsonb
			WHERE session_id=$1 AND segment_id=$2
		`, sessionID, uniqueIDs[0], strings.TrimSpace(text), startMS, endMS)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		if len(uniqueIDs) == 1 {
			return nil
		}
		tag, err = tx.Exec(ctx, `
			DELETE FROM live_transcript_segments
			WHERE session_id=$1 AND segment_id=ANY($2::text[])
		`, sessionID, uniqueIDs[1:])
		if err != nil {
			return err
		}
		if tag.RowsAffected() != int64(len(uniqueIDs)-1) {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (s *Store) ListLiveSegments(
	ctx context.Context,
	sessionID string,
) ([]domain.TranscriptSegment, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT segment_id, text, start_ms, end_ms, words, command
		FROM live_transcript_segments
		WHERE session_id=$1
		ORDER BY start_ms, created_at
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	segments := make([]domain.TranscriptSegment, 0)
	for rows.Next() {
		var segment domain.TranscriptSegment
		var wordsJSON []byte
		var commandJSON []byte
		if err := rows.Scan(
			&segment.ID,
			&segment.Text,
			&segment.StartMS,
			&segment.EndMS,
			&wordsJSON,
			&commandJSON,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(wordsJSON, &segment.Words); err != nil {
			return nil, err
		}
		if len(commandJSON) > 0 && string(commandJSON) != "null" {
			if err := json.Unmarshal(commandJSON, &segment.Command); err != nil {
				return nil, err
			}
		}
		segments = append(segments, segment)
	}
	return segments, rows.Err()
}

func (s *Store) CreateTranscriptArtifact(
	ctx context.Context,
	artifact domain.TranscriptArtifact,
) (domain.TranscriptArtifact, error) {
	if artifact.ID == "" {
		artifact.ID = uuid.NewString()
	}
	segments, err := json.Marshal(artifact.Segments)
	if err != nil {
		return domain.TranscriptArtifact{}, err
	}
	err = s.Pool.QueryRow(ctx, `
		INSERT INTO transcript_artifacts(
			id, session_id, kind, pipeline_version, provider, model, language,
			text, segments, audio_sha256
		)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT DO NOTHING
		RETURNING id::text, created_at
	`, artifact.ID, artifact.SessionID, artifact.Kind, artifact.PipelineVersion, artifact.Provider,
		artifact.Model, artifact.Language, artifact.Text, segments, artifact.AudioSHA256).Scan(
		&artifact.ID,
		&artifact.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) &&
		(artifact.Kind == "live_final" || artifact.Kind == "batch_source") {
		err = s.Pool.QueryRow(ctx, `
			SELECT id::text, created_at
			FROM transcript_artifacts
			WHERE session_id=$1 AND kind=$2
		`, artifact.SessionID, artifact.Kind).Scan(&artifact.ID, &artifact.CreatedAt)
	}
	return artifact, err
}

func (s *Store) QueueRetranscription(
	ctx context.Context,
	ownerID, sessionID, requestID string,
) error {
	payload, _ := json.Marshal(map[string]string{"sessionId": sessionID})
	tag, err := s.Pool.Exec(ctx, `
		INSERT INTO background_jobs(id, kind, status, idempotency_key, payload)
		SELECT $3, 'retranscribe_dictation', 'queued', $4, $5
		FROM dictation_sessions session
		JOIN audio_assets audio ON audio.session_id=session.id
		WHERE session.id=$1 AND session.owner_user_id=$2
		  AND session.status='review_ready'
		ON CONFLICT(idempotency_key) DO NOTHING
	`, sessionID, ownerID, requestID, "retranscribe:"+requestID, payload)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) GetLatestRetranscriptionStatus(
	ctx context.Context,
	ownerID, sessionID string,
) (string, error) {
	var status string
	err := s.Pool.QueryRow(ctx, `
		SELECT job.status
		FROM background_jobs job
		JOIN dictation_sessions session
		  ON session.id=(job.payload->>'sessionId')::uuid
		WHERE session.id=$1 AND session.owner_user_id=$2
		  AND job.kind='retranscribe_dictation'
		ORDER BY job.created_at DESC
		LIMIT 1
	`, sessionID, ownerID).Scan(&status)
	return status, err
}

func (s *Store) ListTranscriptArtifacts(
	ctx context.Context,
	ownerID, sessionID string,
) ([]domain.TranscriptArtifact, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT a.id::text, a.session_id::text, a.kind, a.pipeline_version, a.provider,
			a.model, a.language, a.text, a.segments, a.audio_sha256, a.created_at
		FROM transcript_artifacts a
		JOIN dictation_sessions s ON s.id=a.session_id
		WHERE a.session_id=$1 AND s.owner_user_id=$2
		ORDER BY a.created_at
	`, sessionID, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	artifacts := make([]domain.TranscriptArtifact, 0)
	for rows.Next() {
		var artifact domain.TranscriptArtifact
		var segmentsJSON []byte
		if err := rows.Scan(
			&artifact.ID,
			&artifact.SessionID,
			&artifact.Kind,
			&artifact.PipelineVersion,
			&artifact.Provider,
			&artifact.Model,
			&artifact.Language,
			&artifact.Text,
			&segmentsJSON,
			&artifact.AudioSHA256,
			&artifact.CreatedAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(segmentsJSON, &artifact.Segments); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, rows.Err()
}

func (s *Store) UpsertTranscriptDraft(
	ctx context.Context,
	draft domain.TranscriptDraft,
) (domain.TranscriptDraft, error) {
	if draft.ID == "" {
		draft.ID = uuid.NewString()
	}
	if draft.Source == "" {
		draft.Source = "Ручная правка"
	}
	backgroundSource := draft.Source == "live_source" || draft.Source == "batch_source"
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO transcript_drafts(
				id, session_id, source_artifact_id, revision, base_source_text,
				content, plain_text, manually_edited
			)
			VALUES($1, $2, $3, 1, $4, $5, $6, $7)
			ON CONFLICT(session_id) DO UPDATE
			SET source_artifact_id=COALESCE(transcript_drafts.source_artifact_id, EXCLUDED.source_artifact_id),
				revision=CASE
					WHEN transcript_drafts.manually_edited AND $8 THEN transcript_drafts.revision
					ELSE transcript_drafts.revision + 1
				END,
				base_source_text=CASE
					WHEN transcript_drafts.manually_edited AND $8 THEN transcript_drafts.base_source_text
					ELSE EXCLUDED.base_source_text
				END,
				content=CASE
					WHEN transcript_drafts.manually_edited AND $8 THEN transcript_drafts.content
					ELSE EXCLUDED.content
				END,
				plain_text=CASE
					WHEN transcript_drafts.manually_edited AND $8 THEN transcript_drafts.plain_text
					ELSE EXCLUDED.plain_text
				END,
				manually_edited=transcript_drafts.manually_edited OR EXCLUDED.manually_edited,
				updated_at=CASE
					WHEN transcript_drafts.manually_edited AND $8 THEN transcript_drafts.updated_at
					ELSE now()
				END
			RETURNING id::text, revision, content, plain_text, created_at, updated_at
		`, draft.ID, draft.SessionID, draft.SourceArtifactID, draft.BaseSourceText, draft.Content,
			draft.PlainText, draft.ManuallyEdited, backgroundSource).Scan(
			&draft.ID,
			&draft.Revision,
			&draft.Content,
			&draft.PlainText,
			&draft.CreatedAt,
			&draft.UpdatedAt,
		); err != nil {
			return err
		}
		if backgroundSource && draft.ManuallyEdited {
			return nil
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO transcript_draft_revisions(
				id, session_id, revision, content, plain_text, source
			)
			VALUES($1, $2, $3, $4, $5, $6)
			ON CONFLICT(session_id, revision) DO NOTHING
		`, uuid.NewString(), draft.SessionID, draft.Revision, draft.Content, draft.PlainText,
			draft.Source)
		return err
	})
	return draft, err
}

func (s *Store) GetTranscriptDraft(
	ctx context.Context,
	ownerID, sessionID string,
) (domain.TranscriptDraft, error) {
	var draft domain.TranscriptDraft
	err := s.Pool.QueryRow(ctx, `
		SELECT d.id::text, d.session_id::text, d.source_artifact_id::text, d.revision,
			d.base_source_text, d.content, d.plain_text, d.manually_edited, d.created_at, d.updated_at
		FROM transcript_drafts d
		JOIN dictation_sessions s ON s.id=d.session_id
		WHERE d.session_id=$1 AND s.owner_user_id=$2
	`, sessionID, ownerID).Scan(
		&draft.ID,
		&draft.SessionID,
		&draft.SourceArtifactID,
		&draft.Revision,
		&draft.BaseSourceText,
		&draft.Content,
		&draft.PlainText,
		&draft.ManuallyEdited,
		&draft.CreatedAt,
		&draft.UpdatedAt,
	)
	return draft, err
}

func (s *Store) ListTranscriptDraftRevisions(
	ctx context.Context,
	ownerID, sessionID string,
) ([]domain.TranscriptDraftRevision, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT revision.id::text, revision.session_id::text, revision.revision,
			revision.content, revision.plain_text, revision.source, revision.created_at
		FROM transcript_draft_revisions revision
		JOIN dictation_sessions session ON session.id=revision.session_id
		WHERE revision.session_id=$1 AND session.owner_user_id=$2
		ORDER BY revision.revision DESC
	`, sessionID, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	revisions := make([]domain.TranscriptDraftRevision, 0)
	for rows.Next() {
		var revision domain.TranscriptDraftRevision
		if err := rows.Scan(
			&revision.ID,
			&revision.SessionID,
			&revision.Revision,
			&revision.Content,
			&revision.PlainText,
			&revision.Source,
			&revision.CreatedAt,
		); err != nil {
			return nil, err
		}
		revisions = append(revisions, revision)
	}
	return revisions, rows.Err()
}

func (s *Store) ListRecoveryItems(ctx context.Context, ownerID string) ([]domain.RecoveryItem, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT s.id::text, s.project_id::text, p.title, n.title, s.status,
			s.recorded_duration_ms, count(c.sequence)::int, count(c.sequence)::int, s.created_at
		FROM dictation_sessions s
		JOIN projects p ON p.id=s.project_id
		LEFT JOIN document_nodes n ON n.id=s.destination_node_id
		LEFT JOIN audio_chunks c ON c.session_id=s.id
		WHERE s.owner_user_id=$1
		  AND s.status IN ('recording_online','recording_degraded','paused','stopping','finalizing','finalization_failed','missing_chunks')
		GROUP BY s.id, p.title, n.title
		ORDER BY s.created_at DESC
	`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.RecoveryItem, 0)
	for rows.Next() {
		var item domain.RecoveryItem
		if err := rows.Scan(
			&item.SessionID,
			&item.ProjectID,
			&item.ProjectTitle,
			&item.DestinationTitle,
			&item.Status,
			&item.DurationMS,
			&item.LocalChunkCount,
			&item.RemoteChunkCount,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ClaimBackgroundJob(ctx context.Context) (BackgroundJob, error) {
	var job BackgroundJob
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			WITH next_job AS (
				SELECT id
				FROM background_jobs
				WHERE status='queued' AND available_at <= now()
				ORDER BY created_at
				FOR UPDATE SKIP LOCKED
				LIMIT 1
			)
			UPDATE background_jobs j
			SET status='running', attempts=j.attempts+1, updated_at=now()
			FROM next_job
			WHERE j.id=next_job.id
			RETURNING j.id::text, j.kind, j.status, j.idempotency_key, j.payload, j.attempts
		`).Scan(&job.ID, &job.Kind, &job.Status, &job.IdempotencyKey, &job.Payload, &job.Attempts)
	})
	return job, err
}

func (s *Store) CompleteBackgroundJob(
	ctx context.Context,
	jobID string,
	result json.RawMessage,
) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE background_jobs
		SET status='ready', result=$2, error_code=NULL, updated_at=now()
		WHERE id=$1
	`, jobID, result)
	return err
}

func (s *Store) FailBackgroundJobOnly(ctx context.Context, jobID, errorCode string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE background_jobs
		SET status='failed', error_code=$2, updated_at=now()
		WHERE id=$1
	`, jobID, errorCode)
	return err
}

func (s *Store) FailBackgroundJob(
	ctx context.Context,
	jobID, sessionID, errorCode, sessionStatus string,
) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE background_jobs
			SET status='failed', error_code=$2, updated_at=now()
			WHERE id=$1
		`, jobID, errorCode); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			UPDATE dictation_sessions
			SET status=$2, updated_at=now()
			WHERE id=$1
		`, sessionID, sessionStatus)
		return err
	})
}

func (s *Store) RetryDictation(ctx context.Context, ownerID, sessionID string) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE dictation_sessions
			SET status='finalizing', updated_at=now()
			WHERE id=$1 AND owner_user_id=$2
			  AND status IN ('finalization_failed','missing_chunks')
		`, sessionID, ownerID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		payload, _ := json.Marshal(map[string]string{"sessionId": sessionID})
		_, err = tx.Exec(ctx, `
			INSERT INTO background_jobs(id, kind, status, idempotency_key, payload)
			VALUES($1, 'finalize_dictation', 'queued', $2, $3)
			ON CONFLICT(idempotency_key) DO UPDATE
			SET status='queued', available_at=now(), error_code=NULL, updated_at=now()
		`, uuid.NewString(), "finalize:"+sessionID, payload)
		return err
	})
}

func (s *Store) SaveAudioAsset(ctx context.Context, asset AudioAsset) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO audio_assets(id, session_id, sha256, storage_key, byte_size, duration_ms)
		VALUES($1, $2, $3, $4, $5, $6)
		ON CONFLICT(session_id) DO NOTHING
	`, asset.ID, asset.SessionID, asset.SHA256, asset.StorageKey, asset.ByteSize, asset.DurationMS)
	return err
}

func (s *Store) GetAudioAsset(ctx context.Context, ownerID, sessionID string) (AudioAsset, error) {
	var asset AudioAsset
	err := s.Pool.QueryRow(ctx, `
		SELECT a.id::text, a.session_id::text, a.sha256, a.storage_key, a.byte_size, a.duration_ms
		FROM audio_assets a
		JOIN dictation_sessions s ON s.id=a.session_id
		WHERE a.session_id=$1 AND s.owner_user_id=$2
	`, sessionID, ownerID).Scan(
		&asset.ID,
		&asset.SessionID,
		&asset.SHA256,
		&asset.StorageKey,
		&asset.ByteSize,
		&asset.DurationMS,
	)
	return asset, err
}

func (s *Store) GetAudioAssetForWorker(ctx context.Context, sessionID string) (AudioAsset, error) {
	var asset AudioAsset
	err := s.Pool.QueryRow(ctx, `
		SELECT id::text, session_id::text, sha256, storage_key, byte_size, duration_ms
		FROM audio_assets
		WHERE session_id=$1
	`, sessionID).Scan(
		&asset.ID,
		&asset.SessionID,
		&asset.SHA256,
		&asset.StorageKey,
		&asset.ByteSize,
		&asset.DurationMS,
	)
	return asset, err
}

func (s *Store) GetSegmentAudioSource(
	ctx context.Context,
	ownerID, segmentID string,
) (SegmentAudioSource, error) {
	var source SegmentAudioSource
	err := s.Pool.QueryRow(ctx, `
		SELECT s.id::text, (segment->>'startMs')::bigint, (segment->>'endMs')::bigint
		FROM transcript_artifacts artifact
		JOIN dictation_sessions s ON s.id=artifact.session_id
		JOIN audio_assets audio ON audio.session_id=s.id
		CROSS JOIN LATERAL jsonb_array_elements(artifact.segments) AS segment
		WHERE s.owner_user_id=$1 AND segment->>'id'=$2
		ORDER BY CASE artifact.kind WHEN 'batch_source' THEN 0 ELSE 1 END
		LIMIT 1
	`, ownerID, segmentID).Scan(&source.SessionID, &source.StartMS, &source.EndMS)
	return source, err
}

func (s *Store) FinalizeDictationState(
	ctx context.Context,
	sessionID, status string,
) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE dictation_sessions SET status=$2, updated_at=now() WHERE id=$1
	`, sessionID, status)
	return err
}

type WorkerSessionMeta struct {
	OwnerID            string
	ProjectID          string
	DestinationNodeID  *string
	Language           string
	DurationMS         int64
	RecordedDurationMS int64
	Manifest           json.RawMessage
	LiveFinalizedAt    *time.Time
	CommandsEnabled    bool
	CommandPrefix      string
}

func (s *Store) GetWorkerSessionMeta(ctx context.Context, sessionID string) (WorkerSessionMeta, error) {
	var meta WorkerSessionMeta
	err := s.Pool.QueryRow(ctx, `
		SELECT owner_user_id::text, project_id::text, destination_node_id::text,
			language, duration_ms, recorded_duration_ms, manifest, live_finalized_at,
			commands_enabled, command_prefix
		FROM dictation_sessions
		WHERE id=$1
	`, sessionID).Scan(
		&meta.OwnerID,
		&meta.ProjectID,
		&meta.DestinationNodeID,
		&meta.Language,
		&meta.DurationMS,
		&meta.RecordedDurationMS,
		&meta.Manifest,
		&meta.LiveFinalizedAt,
		&meta.CommandsEnabled,
		&meta.CommandPrefix,
	)
	return meta, err
}

func validDictationTransition(current, next string) bool {
	allowed := map[string][]string{
		"recording_online":   {"paused", "recording_degraded", "stopping"},
		"recording_degraded": {"recording_online", "paused", "stopping"},
		"paused":             {"recording_online", "recording_degraded", "stopping"},
	}
	for _, candidate := range allowed[current] {
		if candidate == next {
			return true
		}
	}
	return current == next
}

func SortSegments(segments []domain.TranscriptSegment) {
	sort.SliceStable(segments, func(i, j int) bool {
		return segments[i].StartMS < segments[j].StartMS
	})
}

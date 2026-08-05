DROP INDEX IF EXISTS transcript_artifacts_source_kind_idx;

CREATE UNIQUE INDEX IF NOT EXISTS transcript_artifacts_immutable_source_kind_idx
    ON transcript_artifacts(session_id, kind)
    WHERE kind IN ('live_final', 'batch_source');


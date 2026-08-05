CREATE TABLE IF NOT EXISTS transcript_draft_revisions (
    id uuid PRIMARY KEY,
    session_id uuid NOT NULL REFERENCES dictation_sessions(id) ON DELETE CASCADE,
    revision integer NOT NULL,
    content jsonb NOT NULL,
    plain_text text NOT NULL,
    source text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(session_id, revision)
);

CREATE INDEX IF NOT EXISTS transcript_draft_revisions_session_idx
    ON transcript_draft_revisions(session_id, revision DESC);

INSERT INTO transcript_draft_revisions(id, session_id, revision, content, plain_text, source, created_at)
SELECT gen_random_uuid(), session_id, revision, content, plain_text, 'Импортирован текущий черновик', created_at
FROM transcript_drafts
ON CONFLICT(session_id, revision) DO NOTHING;

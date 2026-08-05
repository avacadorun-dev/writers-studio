CREATE TABLE IF NOT EXISTS schema_migrations (
    version integer PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
    id uuid PRIMARY KEY,
    email text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    display_name text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS auth_sessions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash text NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS projects (
    id uuid PRIMARY KEY,
    owner_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title varchar(120) NOT NULL,
    kind text NOT NULL CHECK (kind IN ('novel', 'story', 'screenplay', 'nonfiction', 'free')),
    language text NOT NULL CHECK (language IN ('ru', 'en')),
    synopsis text NOT NULL DEFAULT '',
    word_goal integer,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived', 'trash')),
    trashed_at timestamptz,
    purge_after timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (word_goal IS NULL OR word_goal BETWEEN 1 AND 10000000)
);
CREATE INDEX IF NOT EXISTS projects_owner_status_idx ON projects(owner_user_id, status, updated_at DESC);

CREATE TABLE IF NOT EXISTS document_nodes (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    parent_id uuid REFERENCES document_nodes(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('part', 'chapter', 'scene', 'draft', 'note')),
    title varchar(200) NOT NULL,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('idea', 'draft', 'editing', 'ready')),
    position numeric(20, 10) NOT NULL,
    current_revision integer NOT NULL DEFAULT 0,
    trashed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS document_nodes_project_parent_idx ON document_nodes(project_id, parent_id, position);

CREATE TABLE IF NOT EXISTS document_revisions (
    id uuid PRIMARY KEY,
    node_id uuid NOT NULL REFERENCES document_nodes(id) ON DELETE CASCADE,
    revision integer NOT NULL,
    base_revision integer NOT NULL,
    schema_version integer NOT NULL DEFAULT 1,
    content jsonb NOT NULL,
    plain_text text NOT NULL DEFAULT '',
    source text NOT NULL DEFAULT 'manual',
    operation_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(node_id, revision)
);
CREATE INDEX IF NOT EXISTS document_revisions_node_created_idx ON document_revisions(node_id, created_at DESC);
CREATE INDEX IF NOT EXISTS document_revisions_search_idx ON document_revisions USING gin(to_tsvector('simple', plain_text));

CREATE TABLE IF NOT EXISTS named_versions (
    id uuid PRIMARY KEY,
    node_id uuid NOT NULL REFERENCES document_nodes(id) ON DELETE CASCADE,
    revision integer NOT NULL,
    name varchar(160) NOT NULL,
    note text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(node_id, id)
);

CREATE TABLE IF NOT EXISTS dictionary_terms (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    canonical text NOT NULL,
    variants jsonb NOT NULL DEFAULT '[]'::jsonb,
    kind text NOT NULL CHECK (kind IN ('character', 'place', 'term', 'other')),
    language text NOT NULL CHECK (language IN ('ru', 'en')),
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS dictionary_terms_project_idx ON dictionary_terms(project_id, active, canonical);

CREATE TABLE IF NOT EXISTS dictation_sessions (
    id uuid PRIMARY KEY,
    owner_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    destination_node_id uuid REFERENCES document_nodes(id) ON DELETE SET NULL,
    client_session_id text NOT NULL,
    language text NOT NULL CHECK (language IN ('ru', 'en')),
    status text NOT NULL CHECK (status IN (
        'preparing', 'recording_online', 'recording_degraded', 'paused',
        'stopping', 'finalizing', 'review_ready', 'finalization_failed', 'missing_chunks', 'trashed'
    )),
    duration_ms bigint NOT NULL DEFAULT 0,
    recorded_duration_ms bigint NOT NULL DEFAULT 0,
    manifest jsonb NOT NULL DEFAULT '{"chunks":[]}'::jsonb,
    stopped_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(owner_user_id, client_session_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS dictation_one_active_client_idx
    ON dictation_sessions(owner_user_id, client_session_id)
    WHERE status IN ('preparing', 'recording_online', 'recording_degraded', 'paused', 'stopping');

CREATE TABLE IF NOT EXISTS audio_chunks (
    session_id uuid NOT NULL REFERENCES dictation_sessions(id) ON DELETE CASCADE,
    sequence integer NOT NULL,
    sha256 char(64) NOT NULL,
    duration_ms integer NOT NULL,
    storage_key text NOT NULL,
    byte_size bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(session_id, sequence)
);

CREATE TABLE IF NOT EXISTS audio_assets (
    id uuid PRIMARY KEY,
    session_id uuid NOT NULL UNIQUE REFERENCES dictation_sessions(id) ON DELETE CASCADE,
    sha256 char(64) NOT NULL,
    storage_key text NOT NULL,
    byte_size bigint NOT NULL,
    duration_ms bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS transcript_artifacts (
    id uuid PRIMARY KEY,
    session_id uuid NOT NULL REFERENCES dictation_sessions(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('live_final', 'batch_source', 'cleaned_suggestion')),
    pipeline_version text NOT NULL,
    provider text NOT NULL,
    model text NOT NULL,
    language text NOT NULL,
    text text NOT NULL,
    segments jsonb NOT NULL DEFAULT '[]'::jsonb,
    audio_sha256 char(64),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS transcript_artifacts_session_idx ON transcript_artifacts(session_id, created_at);
CREATE UNIQUE INDEX IF NOT EXISTS transcript_artifacts_source_kind_idx
    ON transcript_artifacts(session_id, kind);

CREATE TABLE IF NOT EXISTS live_transcript_segments (
    session_id uuid NOT NULL REFERENCES dictation_sessions(id) ON DELETE CASCADE,
    segment_id text NOT NULL,
    provider_turn_id text NOT NULL,
    text text NOT NULL,
    start_ms bigint NOT NULL,
    end_ms bigint NOT NULL,
    words jsonb NOT NULL DEFAULT '[]'::jsonb,
    command jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(session_id, segment_id)
);

CREATE TABLE IF NOT EXISTS transcript_drafts (
    id uuid PRIMARY KEY,
    session_id uuid NOT NULL UNIQUE REFERENCES dictation_sessions(id) ON DELETE CASCADE,
    source_artifact_id uuid REFERENCES transcript_artifacts(id) ON DELETE SET NULL,
    revision integer NOT NULL DEFAULT 1,
    base_source_text text NOT NULL DEFAULT '',
    content jsonb NOT NULL,
    plain_text text NOT NULL,
    manually_edited boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS background_jobs (
    id uuid PRIMARY KEY,
    kind text NOT NULL,
    status text NOT NULL CHECK (status IN ('queued', 'running', 'ready', 'failed')),
    idempotency_key text NOT NULL UNIQUE,
    payload jsonb NOT NULL,
    result jsonb,
    error_code text,
    attempts integer NOT NULL DEFAULT 0,
    available_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS background_jobs_ready_idx ON background_jobs(status, available_at);

CREATE TABLE IF NOT EXISTS export_jobs (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    owner_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    format text NOT NULL CHECK (format IN ('docx', 'markdown', 'txt')),
    status text NOT NULL CHECK (status IN ('queued', 'running', 'ready', 'failed')),
    snapshot jsonb NOT NULL,
    storage_key text,
    error_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

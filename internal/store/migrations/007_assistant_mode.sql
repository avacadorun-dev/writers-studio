ALTER TABLE document_revisions
    ADD COLUMN IF NOT EXISTS assistant_run_id uuid,
    ADD COLUMN IF NOT EXISTS assistant_change_set_id uuid;

ALTER TABLE dictionary_terms
    ADD COLUMN IF NOT EXISTS pronunciation text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS revision integer NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS source text NOT NULL DEFAULT 'manual',
    ADD COLUMN IF NOT EXISTS operation_id uuid,
    ADD COLUMN IF NOT EXISTS assistant_run_id uuid;

CREATE TABLE IF NOT EXISTS assistant_provider_connections (
    id uuid PRIMARY KEY,
    owner_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name varchar(120) NOT NULL,
    base_url text NOT NULL,
    dialect text NOT NULL CHECK (dialect IN ('responses', 'chat_completions', 'auto')),
    model varchar(240) NOT NULL,
    encrypted_api_key bytea NOT NULL,
    api_key_nonce bytea NOT NULL,
    api_key_fingerprint char(64) NOT NULL,
    api_key_last4 varchar(4) NOT NULL DEFAULT '',
    encrypted_headers bytea,
    headers_nonce bytea,
    request_timeout_ms integer NOT NULL DEFAULT 120000
        CHECK (request_timeout_ms BETWEEN 1000 AND 600000),
    context_window integer CHECK (context_window IS NULL OR context_window > 0),
    allow_private_endpoint boolean NOT NULL DEFAULT false,
    capabilities jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'unchecked'
        CHECK (status IN ('unchecked', 'checking', 'compatible', 'limited', 'unavailable', 'disabled')),
    status_detail text NOT NULL DEFAULT '',
    is_default boolean NOT NULL DEFAULT false,
    checked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(owner_user_id, name)
);
CREATE UNIQUE INDEX IF NOT EXISTS assistant_provider_one_default_idx
    ON assistant_provider_connections(owner_user_id)
    WHERE is_default AND status <> 'disabled';

CREATE TABLE IF NOT EXISTS assistant_guides (
    project_id uuid PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    current_revision integer NOT NULL DEFAULT 0,
    updated_by_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS assistant_guide_revisions (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES assistant_guides(project_id) ON DELETE CASCADE,
    revision integer NOT NULL,
    content text NOT NULL CHECK (char_length(content) <= 24000),
    source text NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'assistant', 'restore')),
    restored_from_revision integer,
    assistant_run_id uuid,
    created_by_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(project_id, revision)
);
CREATE INDEX IF NOT EXISTS assistant_guide_revisions_project_idx
    ON assistant_guide_revisions(project_id, revision DESC);

CREATE TABLE IF NOT EXISTS project_memory_records (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN (
        'fact', 'character', 'world_rule', 'plot', 'style', 'constraint', 'decision', 'other'
    )),
    statement text NOT NULL CHECK (char_length(statement) BETWEEN 1 AND 12000),
    scope text NOT NULL DEFAULT 'project' CHECK (scope IN ('project', 'node')),
    scope_node_id uuid REFERENCES document_nodes(id) ON DELETE SET NULL,
    source_references jsonb NOT NULL DEFAULT '[]'::jsonb,
    tags jsonb NOT NULL DEFAULT '[]'::jsonb,
    active boolean NOT NULL DEFAULT true,
    always_relevant boolean NOT NULL DEFAULT false,
    revision integer NOT NULL DEFAULT 1,
    source text NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'assistant')),
    operation_id uuid,
    assistant_run_id uuid,
    created_by_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (
        (scope = 'project' AND scope_node_id IS NULL)
        OR (scope = 'node' AND scope_node_id IS NOT NULL)
    )
);
CREATE INDEX IF NOT EXISTS project_memory_project_active_idx
    ON project_memory_records(project_id, active, always_relevant DESC, updated_at DESC);
CREATE INDEX IF NOT EXISTS project_memory_search_idx
    ON project_memory_records USING gin(to_tsvector('simple', statement));

CREATE TABLE IF NOT EXISTS project_memory_revisions (
    id uuid PRIMARY KEY,
    memory_id uuid NOT NULL REFERENCES project_memory_records(id) ON DELETE CASCADE,
    revision integer NOT NULL,
    snapshot jsonb NOT NULL,
    source text NOT NULL CHECK (source IN ('manual', 'assistant')),
    operation_id uuid,
    assistant_run_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(memory_id, revision)
);

CREATE TABLE IF NOT EXISTS assistant_runs (
    id uuid PRIMARY KEY,
    owner_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    target_kind text NOT NULL CHECK (target_kind IN ('project', 'node')),
    target_node_id uuid REFERENCES document_nodes(id) ON DELETE SET NULL,
    input_mode text NOT NULL CHECK (input_mode IN ('voice', 'chat')),
    status text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'working', 'awaiting_review', 'completed', 'cancelled', 'failed')),
    silence_checkpoint_ms integer NOT NULL DEFAULT 5000
        CHECK (silence_checkpoint_ms IN (0, 3000, 5000, 8000)),
    provider_kind text NOT NULL CHECK (provider_kind IN ('system', 'connection')),
    provider_connection_id uuid REFERENCES assistant_provider_connections(id) ON DELETE SET NULL,
    provider_name text NOT NULL,
    model text NOT NULL,
    dialect text NOT NULL CHECK (dialect IN ('responses', 'chat_completions')),
    capability_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    guide_revision integer,
    read_context jsonb NOT NULL DEFAULT '{}'::jsonb,
    error_code text,
    started_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    ended_at timestamptz,
    CHECK (
        (target_kind = 'project' AND target_node_id IS NULL)
        OR (target_kind = 'node' AND target_node_id IS NOT NULL)
    ),
    CHECK (
        (provider_kind = 'system' AND provider_connection_id IS NULL)
        OR provider_kind = 'connection'
    )
);
CREATE INDEX IF NOT EXISTS assistant_runs_project_idx
    ON assistant_runs(project_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS assistant_runs_owner_active_idx
    ON assistant_runs(owner_user_id, status, updated_at DESC);

CREATE TABLE IF NOT EXISTS assistant_turns (
    id uuid PRIMARY KEY,
    run_id uuid NOT NULL REFERENCES assistant_runs(id) ON DELETE CASCADE,
    sequence integer NOT NULL,
    input_mode text NOT NULL CHECK (input_mode IN ('voice', 'chat')),
    state text NOT NULL DEFAULT 'collecting' CHECK (state IN (
        'collecting', 'awaiting_confirmation', 'queued', 'working',
        'awaiting_review', 'completed', 'failed', 'cancelled'
    )),
    transcript text NOT NULL DEFAULT '',
    instruction text NOT NULL DEFAULT '',
    assistant_text text NOT NULL DEFAULT '',
    guide_revision integer,
    provider_response_id text NOT NULL DEFAULT '',
    error_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    confirmed_at timestamptz,
    started_at timestamptz,
    completed_at timestamptz,
    UNIQUE(run_id, sequence)
);
CREATE INDEX IF NOT EXISTS assistant_turns_run_idx
    ON assistant_turns(run_id, sequence);

CREATE TABLE IF NOT EXISTS assistant_events (
    id uuid PRIMARY KEY,
    run_id uuid NOT NULL REFERENCES assistant_runs(id) ON DELETE CASCADE,
    turn_id uuid REFERENCES assistant_turns(id) ON DELETE CASCADE,
    sequence bigint NOT NULL,
    event_type text NOT NULL,
    data jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(run_id, sequence)
);
CREATE INDEX IF NOT EXISTS assistant_events_run_sequence_idx
    ON assistant_events(run_id, sequence);

CREATE TABLE IF NOT EXISTS assistant_tool_events (
    id uuid PRIMARY KEY,
    run_id uuid NOT NULL REFERENCES assistant_runs(id) ON DELETE CASCADE,
    turn_id uuid NOT NULL REFERENCES assistant_turns(id) ON DELETE CASCADE,
    call_id text NOT NULL,
    tool_name text NOT NULL,
    arguments jsonb NOT NULL DEFAULT '{}'::jsonb,
    result jsonb,
    is_error boolean NOT NULL DEFAULT false,
    started_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    UNIQUE(run_id, call_id)
);
CREATE INDEX IF NOT EXISTS assistant_tool_events_turn_idx
    ON assistant_tool_events(turn_id, started_at);

CREATE TABLE IF NOT EXISTS assistant_change_sets (
    id uuid PRIMARY KEY,
    run_id uuid NOT NULL REFERENCES assistant_runs(id) ON DELETE CASCADE,
    turn_id uuid NOT NULL REFERENCES assistant_turns(id) ON DELETE CASCADE,
    state text NOT NULL DEFAULT 'staged'
        CHECK (state IN ('staged', 'applying', 'applied', 'rejected', 'stale', 'failed', 'undone')),
    title varchar(240) NOT NULL,
    summary text NOT NULL DEFAULT '',
    risk_level text NOT NULL DEFAULT 'low' CHECK (risk_level IN ('low', 'medium', 'high')),
    base_fingerprint char(64) NOT NULL,
    operation_id uuid,
    applied_at timestamptz,
    rejected_at timestamptz,
    undone_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(turn_id)
);
CREATE INDEX IF NOT EXISTS assistant_change_sets_run_idx
    ON assistant_change_sets(run_id, created_at DESC);

CREATE TABLE IF NOT EXISTS assistant_change_operations (
    id uuid PRIMARY KEY,
    change_set_id uuid NOT NULL REFERENCES assistant_change_sets(id) ON DELETE CASCADE,
    position integer NOT NULL,
    tool_name text NOT NULL,
    target_kind text NOT NULL,
    target_id uuid,
    base_revision integer,
    arguments jsonb NOT NULL DEFAULT '{}'::jsonb,
    before_snapshot jsonb,
    proposed_snapshot jsonb NOT NULL,
    risk_level text NOT NULL DEFAULT 'low' CHECK (risk_level IN ('low', 'medium', 'high')),
    UNIQUE(change_set_id, position)
);

CREATE TABLE IF NOT EXISTS assistant_voice_segments (
    id uuid PRIMARY KEY,
    run_id uuid NOT NULL REFERENCES assistant_runs(id) ON DELETE CASCADE,
    turn_id uuid NOT NULL REFERENCES assistant_turns(id) ON DELETE CASCADE,
    sequence integer NOT NULL,
    provider_turn_id text NOT NULL DEFAULT '',
    text text NOT NULL,
    start_ms bigint NOT NULL DEFAULT 0,
    end_ms bigint NOT NULL DEFAULT 0,
    confidence double precision,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(turn_id, sequence)
);

CREATE TABLE IF NOT EXISTS assistant_idempotency_keys (
    owner_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scope text NOT NULL,
    idempotency_key text NOT NULL,
    response_status integer NOT NULL,
    response_body jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(owner_user_id, scope, idempotency_key)
);

ALTER TABLE document_revisions
    DROP CONSTRAINT IF EXISTS document_revisions_assistant_run_id_fkey,
    ADD CONSTRAINT document_revisions_assistant_run_id_fkey
        FOREIGN KEY (assistant_run_id) REFERENCES assistant_runs(id) ON DELETE SET NULL;

ALTER TABLE document_revisions
    DROP CONSTRAINT IF EXISTS document_revisions_assistant_change_set_id_fkey,
    ADD CONSTRAINT document_revisions_assistant_change_set_id_fkey
        FOREIGN KEY (assistant_change_set_id) REFERENCES assistant_change_sets(id) ON DELETE SET NULL;

ALTER TABLE dictionary_terms
    DROP CONSTRAINT IF EXISTS dictionary_terms_assistant_run_id_fkey,
    ADD CONSTRAINT dictionary_terms_assistant_run_id_fkey
        FOREIGN KEY (assistant_run_id) REFERENCES assistant_runs(id) ON DELETE SET NULL;

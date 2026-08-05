CREATE TABLE IF NOT EXISTS mcp_access_grants (
    id uuid PRIMARY KEY,
    owner_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name varchar(120) NOT NULL,
    client_name varchar(160) NOT NULL DEFAULT '',
    auth_kind text NOT NULL CHECK (auth_kind IN ('manual', 'oauth')),
    token_hash char(64) UNIQUE,
    token_expires_at timestamptz,
    scopes jsonb NOT NULL DEFAULT '[]'::jsonb,
    last_used_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (
        (auth_kind = 'manual' AND token_hash IS NOT NULL AND token_expires_at IS NOT NULL)
        OR (auth_kind = 'manual' AND revoked_at IS NOT NULL AND token_hash IS NULL)
        OR (auth_kind = 'oauth' AND token_hash IS NULL)
    )
);
CREATE INDEX IF NOT EXISTS mcp_access_grants_owner_idx
    ON mcp_access_grants(owner_user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS mcp_grant_projects (
    grant_id uuid NOT NULL REFERENCES mcp_access_grants(id) ON DELETE CASCADE,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    PRIMARY KEY(grant_id, project_id)
);
CREATE INDEX IF NOT EXISTS mcp_grant_projects_project_idx
    ON mcp_grant_projects(project_id, grant_id);

CREATE TABLE IF NOT EXISTS mcp_oauth_clients (
    client_id text PRIMARY KEY,
    client_name varchar(160) NOT NULL,
    redirect_uris jsonb NOT NULL,
    token_endpoint_auth_method text NOT NULL DEFAULT 'none'
        CHECK (token_endpoint_auth_method IN ('none', 'client_secret_post', 'client_secret_basic')),
    client_secret_hash char(64),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS mcp_oauth_codes (
    code_hash char(64) PRIMARY KEY,
    client_id text NOT NULL REFERENCES mcp_oauth_clients(client_id) ON DELETE CASCADE,
    grant_id uuid NOT NULL REFERENCES mcp_access_grants(id) ON DELETE CASCADE,
    redirect_uri text NOT NULL,
    code_challenge text NOT NULL,
    code_challenge_method text NOT NULL DEFAULT 'S256' CHECK (code_challenge_method = 'S256'),
    resource text NOT NULL,
    scopes jsonb NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS mcp_oauth_tokens (
    token_hash char(64) PRIMARY KEY,
    grant_id uuid NOT NULL REFERENCES mcp_access_grants(id) ON DELETE CASCADE,
    client_id text NOT NULL REFERENCES mcp_oauth_clients(client_id) ON DELETE CASCADE,
    token_kind text NOT NULL CHECK (token_kind IN ('access', 'refresh')),
    scopes jsonb NOT NULL,
    audience text NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    last_used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS mcp_oauth_tokens_grant_idx
    ON mcp_oauth_tokens(grant_id, token_kind, expires_at);

CREATE TABLE IF NOT EXISTS mcp_review_tokens (
    token_hash char(64) PRIMARY KEY,
    grant_id uuid NOT NULL REFERENCES mcp_access_grants(id) ON DELETE CASCADE,
    change_set_id uuid REFERENCES assistant_change_sets(id) ON DELETE CASCADE,
    token_kind text NOT NULL CHECK (token_kind IN ('review', 'action')),
    action_kind text,
    expected_state text,
    content_digest char(64) NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS mcp_review_tokens_grant_idx
    ON mcp_review_tokens(grant_id, expires_at);

CREATE TABLE IF NOT EXISTS assistant_turn_drafts (
    turn_id uuid PRIMARY KEY REFERENCES assistant_turns(id) ON DELETE CASCADE,
    run_id uuid NOT NULL REFERENCES assistant_runs(id) ON DELETE CASCADE,
    operations jsonb NOT NULL DEFAULT '[]'::jsonb,
    tool_call_count integer NOT NULL DEFAULT 0 CHECK (tool_call_count >= 0),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS assistant_turn_drafts_run_idx
    ON assistant_turn_drafts(run_id, updated_at);

CREATE TABLE IF NOT EXISTS mcp_audit_events (
    id uuid PRIMARY KEY,
    grant_id uuid REFERENCES mcp_access_grants(id) ON DELETE SET NULL,
    client_name varchar(160) NOT NULL DEFAULT '',
    session_id_hash char(64),
    tool_name text NOT NULL,
    status text NOT NULL,
    duration_ms bigint NOT NULL DEFAULT 0,
    affected_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
    operation_id uuid,
    change_set_id uuid,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS mcp_audit_events_grant_idx
    ON mcp_audit_events(grant_id, created_at DESC);

ALTER TABLE assistant_runs
    ADD COLUMN IF NOT EXISTS origin text NOT NULL DEFAULT 'internal',
    ADD COLUMN IF NOT EXISTS mcp_grant_id uuid REFERENCES mcp_access_grants(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS mcp_client_name varchar(160) NOT NULL DEFAULT '';

ALTER TABLE assistant_runs DROP CONSTRAINT IF EXISTS assistant_runs_origin_check;
ALTER TABLE assistant_runs ADD CONSTRAINT assistant_runs_origin_check
    CHECK (origin IN ('internal', 'remote_mcp'));
ALTER TABLE assistant_runs DROP CONSTRAINT IF EXISTS assistant_runs_provider_kind_check;
ALTER TABLE assistant_runs ADD CONSTRAINT assistant_runs_provider_kind_check
    CHECK (provider_kind IN ('system', 'connection', 'mcp'));
ALTER TABLE assistant_runs DROP CONSTRAINT IF EXISTS assistant_runs_dialect_check;
ALTER TABLE assistant_runs ADD CONSTRAINT assistant_runs_dialect_check
    CHECK (dialect IN ('responses', 'chat_completions', 'mcp'));
ALTER TABLE assistant_runs DROP CONSTRAINT IF EXISTS assistant_runs_input_mode_check;
ALTER TABLE assistant_runs ADD CONSTRAINT assistant_runs_input_mode_check
    CHECK (input_mode IN ('voice', 'chat', 'mcp'));
ALTER TABLE assistant_runs DROP CONSTRAINT IF EXISTS assistant_runs_check1;
ALTER TABLE assistant_runs DROP CONSTRAINT IF EXISTS assistant_runs_provider_check;
ALTER TABLE assistant_runs ADD CONSTRAINT assistant_runs_provider_check CHECK (
    (provider_kind = 'system' AND provider_connection_id IS NULL)
    OR (provider_kind = 'connection' AND provider_connection_id IS NOT NULL)
    OR (provider_kind = 'mcp' AND provider_connection_id IS NULL)
);

ALTER TABLE assistant_turns DROP CONSTRAINT IF EXISTS assistant_turns_input_mode_check;
ALTER TABLE assistant_turns ADD CONSTRAINT assistant_turns_input_mode_check
    CHECK (input_mode IN ('voice', 'chat', 'mcp'));

CREATE INDEX IF NOT EXISTS assistant_runs_mcp_grant_idx
    ON assistant_runs(mcp_grant_id, updated_at DESC)
    WHERE mcp_grant_id IS NOT NULL;

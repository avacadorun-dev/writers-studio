CREATE TABLE IF NOT EXISTS canvas_schemas (
    project_id uuid PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    current_revision integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS canvas_schema_revisions (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    revision integer NOT NULL,
    base_revision integer NOT NULL,
    schema jsonb NOT NULL,
    source text NOT NULL DEFAULT 'manual',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(project_id, revision)
);

CREATE INDEX IF NOT EXISTS canvas_schema_revisions_project_created_idx
    ON canvas_schema_revisions(project_id, created_at DESC);

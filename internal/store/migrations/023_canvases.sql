CREATE TABLE IF NOT EXISTS canvases (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title varchar(160) NOT NULL,
    position numeric(20, 10) NOT NULL,
    current_revision integer NOT NULL DEFAULT 1,
    trashed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS canvases_project_position_idx
    ON canvases(project_id, position, created_at);

CREATE TABLE IF NOT EXISTS canvas_revisions (
    id uuid PRIMARY KEY,
    canvas_id uuid NOT NULL REFERENCES canvases(id) ON DELETE CASCADE,
    revision integer NOT NULL,
    base_revision integer NOT NULL,
    schema_version integer NOT NULL DEFAULT 1,
    content jsonb NOT NULL,
    source text NOT NULL DEFAULT 'manual',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(canvas_id, revision)
);

CREATE INDEX IF NOT EXISTS canvas_revisions_canvas_created_idx
    ON canvas_revisions(canvas_id, created_at DESC);

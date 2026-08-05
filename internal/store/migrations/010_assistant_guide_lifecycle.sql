ALTER TABLE assistant_guides
    ADD COLUMN IF NOT EXISTS active boolean NOT NULL DEFAULT true;

CREATE INDEX IF NOT EXISTS assistant_guides_project_active_idx
    ON assistant_guides(project_id, active);

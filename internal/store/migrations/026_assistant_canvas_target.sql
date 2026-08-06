ALTER TABLE assistant_runs
    ADD COLUMN IF NOT EXISTS target_canvas_id uuid REFERENCES canvases(id) ON DELETE SET NULL;

DO $$
DECLARE
    check_name text;
BEGIN
    FOR check_name IN
        SELECT conname
        FROM pg_constraint
        WHERE conrelid = 'assistant_runs'::regclass
          AND contype = 'c'
          AND pg_get_constraintdef(oid) ILIKE '%target_kind%'
    LOOP
        EXECUTE format('ALTER TABLE assistant_runs DROP CONSTRAINT %I', check_name);
    END LOOP;
END $$;

ALTER TABLE assistant_runs
    ADD CONSTRAINT assistant_runs_target_kind_check
        CHECK (target_kind IN ('project', 'node', 'canvas')),
    ADD CONSTRAINT assistant_runs_target_reference_check
        CHECK (
            (target_kind = 'project' AND target_node_id IS NULL AND target_canvas_id IS NULL)
            OR (target_kind = 'node' AND target_node_id IS NOT NULL AND target_canvas_id IS NULL)
            OR (target_kind = 'canvas' AND target_node_id IS NULL AND target_canvas_id IS NOT NULL)
        );

CREATE INDEX IF NOT EXISTS assistant_runs_canvas_idx
    ON assistant_runs(target_canvas_id, updated_at DESC)
    WHERE target_canvas_id IS NOT NULL;

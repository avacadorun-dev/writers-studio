ALTER TABLE assistant_runs
    ADD COLUMN IF NOT EXISTS title text NOT NULL DEFAULT 'Новый чат';

ALTER TABLE assistant_runs
    ADD COLUMN IF NOT EXISTS last_viewed_at timestamptz NOT NULL DEFAULT now();

UPDATE assistant_runs r
SET title = COALESCE(
    NULLIF((
        SELECT left(regexp_replace(trim(t.instruction), '\s+', ' ', 'g'), 80)
        FROM assistant_turns t
        WHERE t.run_id = r.id AND trim(t.instruction) <> ''
        ORDER BY t.sequence ASC
        LIMIT 1
    ), ''),
    'Новый чат'
)
WHERE trim(r.title) = '' OR r.title = 'Новый чат';

CREATE INDEX IF NOT EXISTS assistant_runs_project_attention_idx
    ON assistant_runs(project_id, status, updated_at DESC);

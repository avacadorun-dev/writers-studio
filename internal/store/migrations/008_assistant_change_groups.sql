ALTER TABLE assistant_change_operations
    ADD COLUMN IF NOT EXISTS group_key text NOT NULL DEFAULT 'main',
    ADD COLUMN IF NOT EXISTS selected boolean NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS dependencies jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS trace jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS applied boolean NOT NULL DEFAULT false;

ALTER TABLE assistant_change_sets
    ADD COLUMN IF NOT EXISTS linked_change_set_id uuid
        REFERENCES assistant_change_sets(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS assistant_change_operations_group_idx
    ON assistant_change_operations(change_set_id, group_key, position);

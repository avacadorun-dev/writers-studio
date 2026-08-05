ALTER TABLE assistant_turns
    ADD COLUMN IF NOT EXISTS selection_attachment jsonb;

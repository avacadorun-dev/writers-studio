ALTER TABLE dictation_sessions
    ADD COLUMN IF NOT EXISTS live_finalized_at timestamptz;

ALTER TABLE dictation_sessions
    ADD COLUMN IF NOT EXISTS commands_enabled boolean NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS command_prefix text NOT NULL DEFAULT 'Команда';

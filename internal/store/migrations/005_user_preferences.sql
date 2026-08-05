CREATE TABLE IF NOT EXISTS user_preferences (
    user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    audio_retention_days integer NOT NULL DEFAULT 365
        CHECK (audio_retention_days IN (30, 90, 365)),
    updated_at timestamptz NOT NULL DEFAULT now()
);


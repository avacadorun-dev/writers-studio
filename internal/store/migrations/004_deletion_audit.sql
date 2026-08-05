CREATE TABLE IF NOT EXISTS deletion_audit_events (
    id uuid PRIMARY KEY,
    owner_user_id uuid NOT NULL,
    subject_type text NOT NULL,
    subject_id uuid NOT NULL,
    event_type text NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS deletion_audit_owner_created_idx
    ON deletion_audit_events(owner_user_id, created_at DESC);


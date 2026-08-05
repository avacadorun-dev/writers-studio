CREATE TABLE IF NOT EXISTS project_media (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    owner_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    storage_key text NOT NULL UNIQUE,
    media_type text NOT NULL CHECK (media_type IN ('image/png', 'image/jpeg', 'image/gif')),
    original_name varchar(255) NOT NULL DEFAULT '',
    sha256 char(64) NOT NULL,
    byte_size bigint NOT NULL CHECK (byte_size > 0 AND byte_size <= 12582912),
    width integer NOT NULL CHECK (width BETWEEN 1 AND 20000),
    height integer NOT NULL CHECK (height BETWEEN 1 AND 20000),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS project_media_project_idx
    ON project_media(project_id, created_at);
CREATE INDEX IF NOT EXISTS project_media_owner_idx
    ON project_media(owner_user_id, created_at);

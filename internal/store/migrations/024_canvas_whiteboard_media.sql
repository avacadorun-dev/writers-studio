ALTER TABLE project_media
    DROP CONSTRAINT IF EXISTS project_media_media_type_check;
ALTER TABLE project_media
    ADD CONSTRAINT project_media_media_type_check
    CHECK (media_type IN ('image/png', 'image/jpeg', 'image/gif', 'application/pdf'));

ALTER TABLE project_media
    DROP CONSTRAINT IF EXISTS project_media_byte_size_check;
ALTER TABLE project_media
    ADD CONSTRAINT project_media_byte_size_check
    CHECK (
        byte_size > 0 AND
        ((media_type LIKE 'image/%' AND byte_size <= 12582912) OR
         (media_type = 'application/pdf' AND byte_size <= 26214400))
    );

ALTER TABLE project_media ALTER COLUMN width DROP NOT NULL;
ALTER TABLE project_media ALTER COLUMN height DROP NOT NULL;
ALTER TABLE project_media
    DROP CONSTRAINT IF EXISTS project_media_width_check;
ALTER TABLE project_media
    DROP CONSTRAINT IF EXISTS project_media_height_check;
ALTER TABLE project_media
    ADD CONSTRAINT project_media_width_check CHECK (width IS NULL OR width BETWEEN 1 AND 20000);
ALTER TABLE project_media
    ADD CONSTRAINT project_media_height_check CHECK (height IS NULL OR height BETWEEN 1 AND 20000);

ALTER TABLE project_media
    ADD COLUMN IF NOT EXISTS page_count integer CHECK (page_count IS NULL OR page_count > 0),
    ADD COLUMN IF NOT EXISTS duration_ms bigint CHECK (duration_ms IS NULL OR duration_ms >= 0);

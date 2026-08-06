ALTER TABLE project_media
    DROP CONSTRAINT IF EXISTS project_media_media_type_check;
ALTER TABLE project_media
    ADD CONSTRAINT project_media_media_type_check
    CHECK (media_type IN (
        'image/png', 'image/jpeg', 'image/gif', 'application/pdf',
        'audio/mpeg', 'audio/mp4', 'audio/ogg', 'audio/wav',
        'video/mp4', 'video/webm'
    ));

ALTER TABLE project_media
    DROP CONSTRAINT IF EXISTS project_media_byte_size_check;
ALTER TABLE project_media
    ADD CONSTRAINT project_media_byte_size_check
    CHECK (
        byte_size > 0 AND (
            (media_type LIKE 'image/%' AND byte_size <= 12582912) OR
            (media_type = 'application/pdf' AND byte_size <= 26214400) OR
            (media_type LIKE 'audio/%' AND byte_size <= 52428800) OR
            (media_type LIKE 'video/%' AND byte_size <= 104857600)
        )
    );

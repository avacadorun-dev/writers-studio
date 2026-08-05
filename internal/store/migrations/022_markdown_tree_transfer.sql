ALTER TABLE document_nodes
    ADD COLUMN IF NOT EXISTS source_frontmatter text NOT NULL DEFAULT '';

ALTER TABLE project_media
    ADD COLUMN IF NOT EXISTS source_path text;

ALTER TABLE export_jobs DROP CONSTRAINT IF EXISTS export_jobs_format_check;
ALTER TABLE export_jobs
    ADD CONSTRAINT export_jobs_format_check
    CHECK (format IN ('docx', 'epub', 'pdf', 'markdown', 'markdown_tree', 'txt'));

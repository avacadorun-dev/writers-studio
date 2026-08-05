ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS export_profile_revision integer NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS export_profile jsonb NOT NULL DEFAULT '{
        "schemaVersion": 1,
        "metadata": {"title":"","subtitle":"","author":"","annotation":""},
        "style": {
            "preset":"book","fontFamily":"literata","fontSizePt":11,
            "lineHeight":1.5,"paragraphMode":"indent","firstLineIndentEm":1.25,
            "paragraphSpaceEm":0,"textAlign":"justify","pageSize":"a5",
            "marginMm":20,"sceneSeparator":"asterisks"
        },
        "titlePage":true,"tableOfContents":true,"chapterTitles":true,
        "chapterPageBreak":true,"includeNotes":false,"lastFormat":"docx"
    }'::jsonb;

ALTER TABLE export_jobs DROP CONSTRAINT IF EXISTS export_jobs_format_check;
ALTER TABLE export_jobs
    ADD CONSTRAINT export_jobs_format_check
    CHECK (format IN ('docx', 'epub', 'pdf', 'markdown', 'txt'));

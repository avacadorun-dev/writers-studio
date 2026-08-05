ALTER TABLE projects
    DROP CONSTRAINT IF EXISTS projects_kind_check,
    ADD COLUMN IF NOT EXISTS structure_revision integer NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS structure_schema jsonb NOT NULL DEFAULT '{
        "schemaVersion": 1,
        "template": "manuscript",
        "rootLabel": "Рукопись",
        "nodeTypes": [
            {
                "key": "part",
                "label": "Часть",
                "pluralLabel": "Части",
                "icon": "folder",
                "contentBearing": false,
                "allowedChildren": ["chapter", "scene", "draft", "note"],
                "canBeRoot": true
            },
            {
                "key": "chapter",
                "label": "Глава",
                "pluralLabel": "Главы",
                "icon": "book-open",
                "contentBearing": true,
                "allowedChildren": ["scene", "draft", "note"],
                "canBeRoot": true
            },
            {
                "key": "scene",
                "label": "Сцена",
                "pluralLabel": "Сцены",
                "icon": "file-text",
                "contentBearing": true,
                "allowedChildren": ["note"],
                "canBeRoot": true
            },
            {
                "key": "draft",
                "label": "Черновик",
                "pluralLabel": "Черновики",
                "icon": "notebook-pen",
                "contentBearing": true,
                "allowedChildren": [],
                "canBeRoot": true
            },
            {
                "key": "note",
                "label": "Заметка",
                "pluralLabel": "Заметки",
                "icon": "sticky-note",
                "contentBearing": true,
                "allowedChildren": [],
                "canBeRoot": true
            }
        ]
    }'::jsonb;

ALTER TABLE document_nodes
    DROP CONSTRAINT IF EXISTS document_nodes_kind_check;

CREATE INDEX IF NOT EXISTS projects_structure_template_idx
    ON projects((structure_schema->>'template'));

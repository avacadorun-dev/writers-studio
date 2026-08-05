CREATE TABLE document_annotations (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    node_id uuid NOT NULL REFERENCES document_nodes(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('dictionary_link', 'note')),
    dictionary_term_id uuid REFERENCES dictionary_terms(id) ON DELETE SET NULL,
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved', 'needs_attention')),
    block_id text NOT NULL,
    start_offset integer NOT NULL CHECK (start_offset >= 0),
    end_offset integer NOT NULL CHECK (end_offset > start_offset),
    exact_text text NOT NULL,
    prefix text NOT NULL DEFAULT '',
    suffix text NOT NULL DEFAULT '',
    note text NOT NULL DEFAULT '',
    target_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    base_revision integer NOT NULL CHECK (base_revision >= 1),
    resolved_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (kind <> 'dictionary_link' OR dictionary_term_id IS NOT NULL OR target_snapshot <> '{}'::jsonb),
    CHECK (kind <> 'note' OR btrim(note) <> '')
);

CREATE INDEX document_annotations_project_status_idx
    ON document_annotations(project_id, status, updated_at DESC);
CREATE INDEX document_annotations_node_status_idx
    ON document_annotations(node_id, status, block_id, start_offset);
CREATE INDEX document_annotations_term_idx
    ON document_annotations(dictionary_term_id)
    WHERE dictionary_term_id IS NOT NULL;

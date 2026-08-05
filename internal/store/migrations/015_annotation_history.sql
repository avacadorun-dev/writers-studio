ALTER TABLE document_annotations
    ADD COLUMN revision integer NOT NULL DEFAULT 1 CHECK (revision >= 1),
    ADD COLUMN anchor_status text NOT NULL DEFAULT 'attached'
        CHECK (anchor_status IN ('attached', 'detached'));

UPDATE document_annotations
SET anchor_status='detached'
WHERE status='needs_attention';

CREATE TABLE document_annotation_history (
    id uuid PRIMARY KEY,
    annotation_id uuid NOT NULL REFERENCES document_annotations(id) ON DELETE CASCADE,
    annotation_revision integer NOT NULL CHECK (annotation_revision >= 2),
    event_kind text NOT NULL CHECK (event_kind IN (
        'assistant_reanchor',
        'assistant_resolve',
        'assistant_detach',
        'assistant_undo'
    )),
    before_snapshot jsonb NOT NULL,
    after_snapshot jsonb NOT NULL,
    source text NOT NULL CHECK (source IN ('assistant', 'assistant_undo')),
    operation_id uuid NOT NULL,
    change_operation_id uuid REFERENCES assistant_change_operations(id) ON DELETE SET NULL,
    assistant_run_id uuid REFERENCES assistant_runs(id) ON DELETE SET NULL,
    assistant_change_set_id uuid REFERENCES assistant_change_sets(id) ON DELETE SET NULL,
    document_revision integer NOT NULL CHECK (document_revision >= 1),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(annotation_id, annotation_revision)
);

CREATE INDEX document_annotation_history_annotation_idx
    ON document_annotation_history(annotation_id, annotation_revision DESC);
CREATE INDEX document_annotation_history_change_set_idx
    ON document_annotation_history(assistant_change_set_id, created_at);
CREATE INDEX document_annotation_history_change_operation_idx
    ON document_annotation_history(change_operation_id, created_at);

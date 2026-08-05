UPDATE project_memory_records
SET source_references = '[]'::jsonb
WHERE source_references = 'null'::jsonb;

UPDATE project_memory_records
SET tags = '[]'::jsonb
WHERE tags = 'null'::jsonb;

UPDATE project_memory_revisions
SET snapshot = jsonb_set(snapshot, '{sourceReferences}', '[]'::jsonb, true)
WHERE snapshot->'sourceReferences' IS NULL
   OR snapshot->'sourceReferences' = 'null'::jsonb;

UPDATE project_memory_revisions
SET snapshot = jsonb_set(snapshot, '{tags}', '[]'::jsonb, true)
WHERE snapshot->'tags' IS NULL
   OR snapshot->'tags' = 'null'::jsonb;

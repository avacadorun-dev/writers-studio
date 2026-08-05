-- One operator-controlled timeout applies to every LLM provider. The column is
-- retained for backwards-compatible API/storage shapes, but runtime provider
-- clients no longer read it as a connection-specific override.
UPDATE assistant_provider_connections
SET request_timeout_ms = 600000
WHERE request_timeout_ms <> 600000;

ALTER TABLE assistant_provider_connections
    ALTER COLUMN request_timeout_ms SET DEFAULT 600000;

COMMENT ON COLUMN assistant_provider_connections.request_timeout_ms IS
    'Compatibility snapshot; effective timeout comes from LLM_REQUEST_TIMEOUT for every provider';

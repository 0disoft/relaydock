-- Persist the complete gateway request/attempt lifecycle and make provider
-- attempts usable even when routes are supplied by a signed snapshot instead
-- of a control.provider_connections row.

ALTER TABLE runtime.requests
    ADD COLUMN IF NOT EXISTS client_request_id text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS tenant_id text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS attempt_count integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS input_tokens bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cache_read_tokens bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cache_write_tokens bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS output_tokens bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS reasoning_tokens bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS error_code text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS error_message text NOT NULL DEFAULT '';

ALTER TABLE runtime.requests
    DROP CONSTRAINT IF EXISTS runtime_requests_attempt_count_check,
    DROP CONSTRAINT IF EXISTS runtime_requests_usage_check;
ALTER TABLE runtime.requests
    ADD CONSTRAINT runtime_requests_attempt_count_check CHECK (attempt_count >= 0),
    ADD CONSTRAINT runtime_requests_usage_check CHECK (
        input_tokens >= 0 AND cache_read_tokens >= 0 AND
        cache_write_tokens >= 0 AND output_tokens >= 0 AND reasoning_tokens >= 0
    );

ALTER TABLE runtime.provider_attempts
    ALTER COLUMN provider_connection_id DROP NOT NULL,
    ADD COLUMN IF NOT EXISTS provider text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS account_id text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS protocol text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS committed boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS error_code text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS error_message text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS runtime_requests_client_request_idx
    ON runtime.requests (project_id, client_request_id, created_at DESC)
    WHERE client_request_id <> '';

CREATE INDEX IF NOT EXISTS runtime_attempts_request_state_idx
    ON runtime.provider_attempts (request_id, state, attempt_number);

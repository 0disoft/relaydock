/* llmnav/1 module
id=relaydock.migration.runtime-journal.down
role=Remove expanded request usage and provider-attempt journal fields while restoring the original connection requirement when data permits.
owns=runtime journal rollback|provider connection constraint restoration
excludes=request row deletion|usage event rollback
search=rollback runtime journal|remove provider attempt provenance|restore provider connection constraint
invariant=The NOT NULL provider connection constraint is restored only when no existing attempt contains NULL.
invariant=Base request and provider-attempt records survive the rollback.
stability=contract
*/

DROP INDEX IF EXISTS runtime.runtime_attempts_request_state_idx;
DROP INDEX IF EXISTS runtime.runtime_requests_client_request_idx;

ALTER TABLE runtime.provider_attempts
    DROP COLUMN IF EXISTS error_message,
    DROP COLUMN IF EXISTS error_code,
    DROP COLUMN IF EXISTS committed,
    DROP COLUMN IF EXISTS protocol,
    DROP COLUMN IF EXISTS account_id,
    DROP COLUMN IF EXISTS provider;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM runtime.provider_attempts WHERE provider_connection_id IS NULL
    ) THEN
        ALTER TABLE runtime.provider_attempts
            ALTER COLUMN provider_connection_id SET NOT NULL;
    END IF;
END $$;

ALTER TABLE runtime.requests
    DROP CONSTRAINT IF EXISTS runtime_requests_usage_check,
    DROP CONSTRAINT IF EXISTS runtime_requests_attempt_count_check,
    DROP COLUMN IF EXISTS error_message,
    DROP COLUMN IF EXISTS error_code,
    DROP COLUMN IF EXISTS reasoning_tokens,
    DROP COLUMN IF EXISTS output_tokens,
    DROP COLUMN IF EXISTS cache_write_tokens,
    DROP COLUMN IF EXISTS cache_read_tokens,
    DROP COLUMN IF EXISTS input_tokens,
    DROP COLUMN IF EXISTS attempt_count,
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS client_request_id;

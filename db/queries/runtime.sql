-- sqlc contracts for ordinary runtime reads and writes. Transactional
-- request/attempt finalization plus outbox enqueue remains in RuntimeJournal.

-- name: CreateRuntimeRequest :execrows
INSERT INTO runtime.requests (
    id, project_id, virtual_key_id, ingress_protocol, virtual_model,
    price_revision_id, authorization_id, state, client_request_id,
    tenant_id, created_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, 'running', $8,
    $9, $10
)
ON CONFLICT (id) DO NOTHING;

-- name: MarkRuntimeRequestCommitted :execrows
UPDATE runtime.requests
SET first_semantic_event_at = COALESCE(first_semantic_event_at, $2)
WHERE id = $1 AND state = 'running';

-- name: CreateProviderAttempt :execrows
INSERT INTO runtime.provider_attempts (
    id, request_id, provider_connection_id, upstream_model, attempt_number,
    state, provider, account_id, protocol, started_at
) VALUES (
    $1, $2, $3, $4, $5,
    'running', $6, $7, $8, $9
)
ON CONFLICT (id) DO NOTHING;

-- name: GetRuntimeRequest :one
SELECT *
FROM runtime.requests
WHERE id = $1;

-- name: ListProviderAttemptsForRequest :many
SELECT *
FROM runtime.provider_attempts
WHERE request_id = $1
ORDER BY attempt_number, id;

-- name: ListUsageForRequest :many
SELECT *
FROM runtime.usage_events
WHERE request_id = $1
ORDER BY created_at, id;

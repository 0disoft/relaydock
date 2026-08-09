-- name: PutContextPack :execrows
INSERT INTO expert.context_packs (
    id, tenant_id, project_id, repository_revision, working_tree_digest,
    manifest, expires_at, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO UPDATE
SET manifest = EXCLUDED.manifest
WHERE expert.context_packs.project_id = EXCLUDED.project_id
  AND expert.context_packs.tenant_id = EXCLUDED.tenant_id
  AND expert.context_packs.manifest = EXCLUDED.manifest;

-- name: GetContextPack :one
SELECT * FROM expert.context_packs WHERE id = $1;

-- name: CreateConsultation :one
INSERT INTO expert.consultations (
    id, tenant_id, project_id, context_pack_id, objective, task_type, route,
    state, maximum_cost_minor, idempotency_key, request_fingerprint,
    available_at, expires_at, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $12, $12)
ON CONFLICT (project_id, idempotency_key) DO NOTHING
RETURNING *;

-- name: GetConsultation :one
SELECT * FROM expert.consultations WHERE id = $1;

-- name: ListConsultations :many
SELECT *
FROM expert.consultations
WHERE project_id = $1
  AND tenant_id = $2
ORDER BY updated_at DESC, id DESC
LIMIT $3;

-- name: ClaimNextConsultation :one
WITH candidate AS (
    SELECT id
    FROM expert.consultations
    WHERE state = 'queued'
      AND route = $1
      AND available_at <= $2
      AND expires_at > $2
      AND attempt_count < $3
    ORDER BY available_at, created_at, id
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
UPDATE expert.consultations AS consultation
SET state = 'running',
    attempt_count = consultation.attempt_count + 1,
    available_at = $2,
    failure_reason = '',
    locked_by = $4,
    locked_at = $2,
    lock_expires_at = $5,
    updated_at = $2
FROM candidate
WHERE consultation.id = candidate.id
RETURNING consultation.*;

-- name: RenewConsultationClaim :one
UPDATE expert.consultations
SET lock_expires_at = $3, updated_at = $2
WHERE id = $1
  AND state = 'running'
  AND locked_by = $4
  AND lock_expires_at > $2
  AND expires_at > $2
RETURNING *;

-- name: RetryConsultationClaim :one
UPDATE expert.consultations
SET state = CASE WHEN attempt_count >= $5 THEN 'failed' ELSE 'queued' END,
    failure_reason = left($4, 2048),
    available_at = CASE WHEN attempt_count >= $5 THEN $2 ELSE $3 END,
    locked_by = NULL,
    locked_at = NULL,
    lock_expires_at = NULL,
    updated_at = $2
WHERE id = $1
  AND state = 'running'
  AND locked_by = $6
  AND lock_expires_at > $2
RETURNING *;

-- name: RecoverStaleConsultationClaims :execrows
UPDATE expert.consultations
SET state = CASE WHEN attempt_count >= $2 THEN 'failed' ELSE 'queued' END,
    failure_reason = 'worker lease expired before completion',
    available_at = $1,
    locked_by = NULL,
    locked_at = NULL,
    lock_expires_at = NULL,
    updated_at = $1
WHERE state = 'running'
  AND lock_expires_at <= $1;

-- name: InsertConsultationResult :exec
INSERT INTO expert.consultation_results (
    id, consultation_id, structured_result, model_attestation, created_at
) VALUES ($1, $2, $3, $4, $5);

-- name: CompleteConsultationClaim :one
UPDATE expert.consultations
SET result_id = $3,
    state = 'completed',
    locked_by = NULL,
    locked_at = NULL,
    lock_expires_at = NULL,
    updated_at = $4
WHERE id = $1
  AND state = 'running'
  AND locked_by = $2
  AND lock_expires_at > $4
RETURNING *;

-- name: FailConsultation :one
UPDATE expert.consultations
SET state = 'failed',
    failure_reason = left($2, 2048),
    locked_by = NULL,
    locked_at = NULL,
    lock_expires_at = NULL,
    updated_at = now()
WHERE id = $1
  AND state NOT IN ('completed', 'failed', 'cancelled', 'expired')
RETURNING *;

-- name: CancelConsultation :one
UPDATE expert.consultations
SET state = 'cancelled',
    locked_by = NULL,
    locked_at = NULL,
    lock_expires_at = NULL,
    updated_at = now()
WHERE id = $1
  AND state IN ('created', 'context_pending', 'approval_pending', 'queued', 'running', 'result_pending')
RETURNING *;

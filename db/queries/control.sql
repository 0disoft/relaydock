-- name: GetRuntimeSnapshot :one
SELECT revision, payload, signature, generated_at, expires_at
FROM control.runtime_snapshots
ORDER BY revision DESC
LIMIT 1;

-- name: GetVirtualKeyByPublicID :one
SELECT id, project_id, public_id, secret_digest, scopes, allowed_models, expires_at, revoked_at
FROM control.virtual_keys
WHERE public_id = $1;

-- name: InsertRuntimeSnapshot :exec
INSERT INTO control.runtime_snapshots (revision, payload, signature, generated_at, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- sqlc contracts for outbox administration. The production worker uses the
-- hand-written repository so all lease transitions stay explicit and fenced.

-- name: EnqueueOutboxEvent :execrows
INSERT INTO outbox.events (id, topic, aggregate_id, payload, available_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO NOTHING;

-- name: ClaimOutboxEvents :many
WITH candidates AS (
    SELECT id
    FROM outbox.events
    WHERE published_at IS NULL
      AND dead_at IS NULL
      AND available_at <= $3
      AND (
          locked_by IS NULL
          OR lock_expires_at IS NULL
          OR lock_expires_at <= $3
      )
    ORDER BY available_at, created_at, id
    FOR UPDATE SKIP LOCKED
    LIMIT $1
)
UPDATE outbox.events AS event
SET locked_by = $2,
    locked_at = $3,
    lock_expires_at = $4
FROM candidates
WHERE event.id = candidates.id
RETURNING event.*;

-- name: MarkOutboxEventPublished :execrows
UPDATE outbox.events
SET published_at = $3,
    locked_by = NULL,
    locked_at = NULL,
    lock_expires_at = NULL,
    last_error = ''
WHERE id = $1
  AND locked_by = $2
  AND lock_expires_at > $3
  AND published_at IS NULL
  AND dead_at IS NULL;

-- name: RequeueOutboxEvent :execrows
UPDATE outbox.events
SET attempts = attempts + 1,
    available_at = $4,
    locked_by = NULL,
    locked_at = NULL,
    lock_expires_at = NULL,
    last_error = left($5, 4096)
WHERE id = $1
  AND locked_by = $2
  AND lock_expires_at > $3
  AND published_at IS NULL
  AND dead_at IS NULL;

-- name: ListOutboxDeadLetters :many
SELECT *
FROM outbox.events
WHERE dead_at IS NOT NULL
ORDER BY dead_at DESC, id DESC
LIMIT $1;

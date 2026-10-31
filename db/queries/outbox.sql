-- name: EnqueueOutboxMessage :one
INSERT INTO outbox_messages (message_type, aggregate_id, routing_key, payload)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ClaimOutboxMessages :many
WITH candidates AS (
    SELECT id FROM outbox_messages
    WHERE published_at IS NULL AND (claimed_until IS NULL OR claimed_until < now())
    ORDER BY created_at
    FOR UPDATE SKIP LOCKED
    LIMIT $1
)
UPDATE outbox_messages AS messages
SET claimed_until = now() + make_interval(secs => $2)
FROM candidates
WHERE messages.id = candidates.id
RETURNING messages.*;

-- name: MarkOutboxMessagePublished :execrows
UPDATE outbox_messages
SET published_at = now(), claimed_until = NULL, last_error = NULL
WHERE id = $1;

-- name: RecordOutboxPublishFailure :execrows
UPDATE outbox_messages
SET attempts = attempts + 1, last_error = $2, claimed_until = NULL
WHERE id = $1;

-- name: CreateEvent :one
INSERT INTO events (application_id, recipient_id, event_type, idempotency_key, payload, request_hash)
VALUES ($1, $2, $3, $4, $5, '')
RETURNING *;

-- name: InsertEventIfAbsent :one
INSERT INTO events (application_id, recipient_id, event_type, idempotency_key, payload, request_hash)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (application_id, idempotency_key) DO NOTHING
RETURNING *;

-- name: GetEventByIdempotencyKey :one
SELECT * FROM events WHERE application_id = $1 AND idempotency_key = $2;

-- name: GetEventByID :one
SELECT * FROM events WHERE id = $1;

-- name: SetEventStatus :execrows
UPDATE events SET status = $2, processed_at = CASE WHEN $2 = 'processed' THEN now() ELSE processed_at END
WHERE id = $1;

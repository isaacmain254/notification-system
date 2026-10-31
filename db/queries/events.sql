-- name: CreateEvent :one
INSERT INTO events (application_id, recipient_id, event_type, idempotency_key, payload)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetEventByID :one
SELECT * FROM events WHERE id = $1;

-- name: SetEventStatus :execrows
UPDATE events SET status = $2, processed_at = CASE WHEN $2 = 'processed' THEN now() ELSE processed_at END
WHERE id = $1;

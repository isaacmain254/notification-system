-- name: CreateTemplate :one
INSERT INTO templates (application_id, event_type, channel, subject, body, is_active)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListActiveTemplatesForEvent :many
SELECT * FROM templates
WHERE application_id = $1 AND event_type = $2 AND is_active = TRUE
ORDER BY channel;

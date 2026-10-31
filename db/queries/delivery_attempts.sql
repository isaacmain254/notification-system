-- name: CreateDeliveryAttempt :one
INSERT INTO delivery_attempts
    (notification_id, attempt_number, status, provider, provider_message_id, error_message)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListDeliveryAttempts :many
SELECT * FROM delivery_attempts WHERE notification_id = $1 ORDER BY attempt_number;

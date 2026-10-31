-- name: CreateNotification :one
INSERT INTO notifications (event_id, recipient_id, channel, status, rendered_subject, rendered_body)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetNotificationByID :one
SELECT * FROM notifications WHERE id = $1;

-- name: ListNotificationsForRecipient :many
SELECT * FROM notifications
WHERE recipient_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: MarkNotificationRead :execrows
UPDATE notifications SET read_at = COALESCE(read_at, now()), updated_at = now()
WHERE id = $1 AND recipient_id = $2;

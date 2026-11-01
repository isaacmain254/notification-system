-- name: UpsertRecipient :one
INSERT INTO recipients (application_id, external_user_id, email, phone)
VALUES ($1, $2, $3, $4)
ON CONFLICT (application_id, external_user_id)
DO UPDATE SET email = COALESCE(EXCLUDED.email, recipients.email),
              phone = COALESCE(EXCLUDED.phone, recipients.phone), updated_at = now()
RETURNING *;

-- name: GetRecipientByExternalID :one
SELECT * FROM recipients WHERE application_id = $1 AND external_user_id = $2;

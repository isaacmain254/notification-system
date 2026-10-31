-- name: CreateApplication :one
INSERT INTO applications (name, api_key_hash)
VALUES ($1, $2)
RETURNING *;

-- name: GetApplicationByID :one
SELECT * FROM applications WHERE id = $1;

-- name: GetActiveApplicationByAPIKeyHash :one
SELECT * FROM applications WHERE api_key_hash = $1 AND is_active = TRUE;

-- name: SetApplicationActive :execrows
UPDATE applications SET is_active = $2, updated_at = now() WHERE id = $1;

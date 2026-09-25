-- name: CreateRefreshToken :exec
INSERT INTO refresh_token (token_hash, staff_id, expires_at) VALUES ($1, $2, $3);

-- name: ConsumeRefreshToken :one
UPDATE refresh_token SET revoked_at = now()
WHERE token_hash = $1 AND revoked_at IS NULL
RETURNING *;

-- name: RevokeRefreshToken :exec
-- Scoped to staff_id so a staff member can only ever revoke their own
-- token: logout must never let one caller revoke someone else's session.
UPDATE refresh_token SET revoked_at = now()
WHERE token_hash = @token_hash AND staff_id = @staff_id AND revoked_at IS NULL;

-- name: RevokeStaffRefreshTokens :exec
UPDATE refresh_token SET revoked_at = now() WHERE staff_id = $1 AND revoked_at IS NULL;

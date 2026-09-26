-- name: GetEndUserByEmail :one
SELECT * FROM end_user WHERE lower(email) = lower($1);

-- name: GetEndUser :one
SELECT * FROM end_user WHERE id = $1;

-- name: CreateEndUser :one
INSERT INTO end_user (email, name) VALUES ($1, $2) RETURNING *;

-- name: UpdateEndUserName :one
UPDATE end_user SET name = $2, updated_at = now() WHERE id = $1 RETURNING *;

-- name: SetEndUserPassword :exec
UPDATE end_user SET password_hash = $2, email_verified_at = COALESCE(email_verified_at, now()), updated_at = now() WHERE id = $1;

-- name: MarkEndUserVerified :exec
UPDATE end_user SET email_verified_at = COALESCE(email_verified_at, now()), updated_at = now() WHERE id = $1;

-- name: CreateClientToken :exec
INSERT INTO client_token (end_user_id, kind, token_hash, ticket_id, expires_at) VALUES ($1, $2, $3, $4, $5);

-- name: ConsumeClientToken :one
UPDATE client_token SET used_at = now()
WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
RETURNING *;

-- name: CreateClientRefreshToken :exec
INSERT INTO client_refresh_token (token_hash, end_user_id, ticket_id, expires_at) VALUES ($1, $2, $3, $4);

-- name: GetClientRefreshToken :one
SELECT * FROM client_refresh_token WHERE token_hash = $1;

-- name: ConsumeClientRefreshToken :one
UPDATE client_refresh_token SET revoked_at = now(), updated_at = now()
WHERE token_hash = $1 AND revoked_at IS NULL
RETURNING *;

-- name: RevokeClientRefreshToken :exec
UPDATE client_refresh_token SET revoked_at = now(), updated_at = now()
WHERE token_hash = $1 AND end_user_id = $2 AND revoked_at IS NULL;

-- name: RevokeClientRefreshTokensForUser :exec
UPDATE client_refresh_token SET revoked_at = now(), updated_at = now() WHERE end_user_id = $1 AND revoked_at IS NULL;

-- name: ClaimTicketUser :exec
UPDATE ticket SET user_id = $2 WHERE id = $1 AND user_id IS NULL;

-- name: SetTicketUser :exec
UPDATE ticket SET user_id = $2 WHERE id = $1;

-- name: ListPortalTickets :many
-- The portal treats resolved like closed: state 'closed' lists both.
SELECT t.id, t.number, t.subject, t.status_id, s.name AS status_name, s.state, d.name AS dept_name,
       t.created_at, t.last_message_at, t.closed_at
FROM ticket t JOIN ticket_status s ON s.id = t.status_id JOIN department d ON d.id = t.dept_id
WHERE t.user_id = @user_id
  AND (sqlc.narg('state')::text IS NULL OR s.state::text = sqlc.narg('state')
       OR (sqlc.narg('state') = 'closed' AND s.state = 'resolved'))
ORDER BY t.last_message_at DESC, t.id DESC
LIMIT @lim OFFSET @off;

-- name: CountPortalTickets :one
SELECT count(*) FROM ticket t JOIN ticket_status s ON s.id = t.status_id
WHERE t.user_id = @user_id
  AND (sqlc.narg('state')::text IS NULL OR s.state::text = sqlc.narg('state')
       OR (sqlc.narg('state') = 'closed' AND s.state = 'resolved'));

-- name: GetPortalTicket :one
SELECT t.id, t.number, t.subject, t.user_id, t.status_id, s.name AS status_name, s.state, d.name AS dept_name,
       h.name AS topic_name, t.created_at, t.updated_at, t.last_message_at, t.closed_at
FROM ticket t JOIN ticket_status s ON s.id = t.status_id JOIN department d ON d.id = t.dept_id
LEFT JOIN help_topic h ON h.id = t.topic_id
WHERE t.id = $1;

-- name: ListPortalThread :many
SELECT e.id, e.type, e.body, e.format, e.created_at, e.user_id, e.staff_id, e.poster,
       u.name AS user_name, st.first_name, st.last_name, st.username
FROM thread_entry e
LEFT JOIN end_user u ON u.id = e.user_id
LEFT JOIN staff st ON st.id = e.staff_id
WHERE e.ticket_id = $1 AND e.type IN ('message', 'response')
ORDER BY e.id;

-- name: SetThreadEntryUser :exec
UPDATE thread_entry SET user_id = $2 WHERE id = $1;

-- name: ListPublicDepartments :many
SELECT id, name FROM department WHERE is_public ORDER BY name;

-- name: ListActiveTopics :many
SELECT id, name FROM help_topic WHERE is_active ORDER BY sort_order, name;

-- name: FirstStatusInState :one
SELECT id, name, state FROM ticket_status WHERE state = $1 ORDER BY sort_order, id LIMIT 1;

-- name: GetTicketIDByNumberAndEmail :one
SELECT id FROM ticket WHERE number = $1 AND lower(requester_email) = lower($2);

-- name: AnnotateLatestTicketEvent :exec
-- Adds the end user who caused it to the ticket's newest event: the status
-- change a portal close, reopen or reply just recorded in the same transaction.
UPDATE ticket_event SET data = data || jsonb_build_object('user_id', @user_id::bigint)
WHERE id = (SELECT max(le.id) FROM ticket_event le WHERE le.ticket_id = @ticket_id::bigint);

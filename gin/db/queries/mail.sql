-- name: ListEmailTemplates :many
SELECT * FROM email_template ORDER BY key;

-- name: GetEmailTemplate :one
SELECT * FROM email_template WHERE key = $1;

-- name: UpdateEmailTemplate :one
UPDATE email_template SET
  subject = COALESCE(sqlc.narg('subject'), subject),
  body_html = COALESCE(sqlc.narg('body_html'), body_html),
  body_text = COALESCE(sqlc.narg('body_text'), body_text),
  updated_at = now()
WHERE key = @key
RETURNING *;

-- name: CreateOutbox :one
INSERT INTO email_outbox (ticket_id, entry_id, template_key, to_address, to_name, subject, body_html, body_text, message_id, in_reply_to, auto_submitted)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id;

-- name: LastSentMessageID :one
SELECT message_id FROM email_outbox
WHERE ticket_id = $1 AND to_address = $2 AND status = 'sent'
ORDER BY sent_at DESC LIMIT 1;

-- name: ClaimOutbox :many
UPDATE email_outbox SET attempts = attempts + 1, updated_at = now()
WHERE id IN (
  SELECT id FROM email_outbox
  WHERE status = 'pending' AND next_attempt_at <= now()
  ORDER BY next_attempt_at
  LIMIT $1
  FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: MarkOutboxSent :exec
UPDATE email_outbox SET status = 'sent', sent_at = now(), last_error = NULL, updated_at = now() WHERE id = $1;

-- name: MarkOutboxFailed :exec
UPDATE email_outbox SET last_error = $2, next_attempt_at = $3, status = $4, updated_at = now() WHERE id = $1;

-- name: ListOutbox :many
SELECT id, ticket_id, entry_id, template_key, to_address, to_name, subject, status, attempts, last_error, next_attempt_at, sent_at, created_at
FROM email_outbox
WHERE (sqlc.narg('status')::email_status IS NULL OR status = sqlc.narg('status')::email_status)
ORDER BY id DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountOutbox :one
SELECT count(*) FROM email_outbox
WHERE (sqlc.narg('status')::email_status IS NULL OR status = sqlc.narg('status')::email_status);

-- name: GetOutbox :one
SELECT * FROM email_outbox WHERE id = $1;

-- name: RetryOutbox :execrows
UPDATE email_outbox SET status = 'pending', attempts = 0, next_attempt_at = now(), last_error = NULL, updated_at = now()
WHERE id = $1 AND status <> 'sent';

-- name: CreateInbound :one
INSERT INTO inbound_message (message_id, from_address, from_name, subject, ticket_id, entry_id, outcome, reason)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetInboundByMessageID :one
SELECT * FROM inbound_message WHERE message_id = $1;

-- name: ListInbound :many
SELECT * FROM inbound_message ORDER BY id DESC LIMIT $1 OFFSET $2;

-- name: CountInbound :one
SELECT count(*) FROM inbound_message;

-- name: ListThreadEntries :many
SELECT * FROM thread_entry
WHERE ticket_id = @ticket_id AND id > @after
ORDER BY id
LIMIT @page_limit::int;

-- name: ListAttachmentsForEntries :many
SELECT a.thread_entry_id, a.file_id, a.inline, f.name, f.mime, f.size
FROM attachment a
JOIN file f ON f.id = a.file_id
WHERE a.thread_entry_id = ANY(@entry_ids::bigint[])
ORDER BY a.thread_entry_id, a.file_id;

-- name: ListTicketEvents :many
SELECT e.id, e.ticket_id, e.staff_id, s.first_name AS staff_first_name, s.last_name AS staff_last_name,
       e.kind, e.data, e.created_at
FROM ticket_event e
LEFT JOIN staff s ON s.id = e.staff_id
WHERE e.ticket_id = $1
ORDER BY e.id;

-- name: SetTicketStatus :exec
-- closed_at is set from the database clock (clock_timestamp(), not now(),
-- for the same reason as CreateTicket) rather than a Go-side time.Now(), so
-- the app server's clock can never disagree with what's stored.
UPDATE ticket
SET status_id = @status_id,
    closed_at = CASE WHEN @close::boolean THEN clock_timestamp() WHEN @reopen::boolean THEN NULL ELSE closed_at END,
    updated_at = now()
WHERE id = @id;

-- name: SetTicketAssignee :exec
UPDATE ticket SET assigned_staff_id = $2, updated_at = now() WHERE id = $1;

-- name: SetTicketDept :exec
UPDATE ticket SET dept_id = $2, updated_at = now() WHERE id = $1;

-- name: MarkTicketAnswered :exec
UPDATE ticket SET is_answered = true, last_response_at = now(), updated_at = now() WHERE id = $1;

-- name: MarkTicketUnanswered :exec
UPDATE ticket SET is_answered = false, last_message_at = clock_timestamp(), updated_at = now() WHERE id = $1;

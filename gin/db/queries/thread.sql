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
UPDATE ticket SET status_id = $2, closed_at = $3, updated_at = now() WHERE id = $1;

-- name: SetTicketAssignee :exec
UPDATE ticket SET assigned_staff_id = $2, updated_at = now() WHERE id = $1;

-- name: SetTicketDept :exec
UPDATE ticket SET dept_id = $2, updated_at = now() WHERE id = $1;

-- name: MarkTicketAnswered :exec
UPDATE ticket SET is_answered = true, last_response_at = now(), updated_at = now() WHERE id = $1;

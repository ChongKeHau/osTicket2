-- name: CreateFile :one
INSERT INTO file (key, name, mime, size, sha256, backend, uploaded_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetFile :one
SELECT * FROM file WHERE id = $1;

-- name: IsFileAttached :one
SELECT EXISTS (SELECT 1 FROM attachment WHERE file_id = $1)::boolean;

-- name: CreateAttachment :exec
INSERT INTO attachment (thread_entry_id, file_id, inline) VALUES ($1, $2, false);

-- name: FileTicketDeptID :one
SELECT t.dept_id
FROM attachment a
JOIN thread_entry te ON te.id = a.thread_entry_id
JOIN ticket t ON t.id = te.ticket_id
WHERE a.file_id = $1
LIMIT 1;

-- name: ListUnattachedFilesBefore :many
SELECT f.* FROM file f
WHERE f.created_at < $1
  AND NOT EXISTS (SELECT 1 FROM attachment a WHERE a.file_id = f.id)
ORDER BY f.id;

-- name: DeleteUnattachedFile :execrows
DELETE FROM file f
WHERE f.id = $1
  AND NOT EXISTS (SELECT 1 FROM attachment a WHERE a.file_id = f.id);

-- name: FileOnCustomerEntry :one
-- Whether the file is attached to a customer-visible entry (a message or a
-- response, never a note) of the ticket.
SELECT EXISTS (
  SELECT 1 FROM attachment a
  JOIN thread_entry te ON te.id = a.thread_entry_id
  WHERE a.file_id = @file_id AND te.ticket_id = @ticket_id AND te.type IN ('message', 'response')
)::boolean;

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

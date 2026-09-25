-- name: ListTopics :many
SELECT * FROM help_topic ORDER BY sort_order, name;

-- name: GetTopic :one
SELECT * FROM help_topic WHERE id = $1;

-- name: CreateTopic :one
INSERT INTO help_topic (name, dept_id, priority_id, is_active, sort_order)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: UpdateTopic :one
UPDATE help_topic
SET name = COALESCE(sqlc.narg('name'), name),
    dept_id = CASE WHEN @clear_dept::boolean THEN NULL ELSE COALESCE(sqlc.narg('dept_id'), dept_id) END,
    priority_id = CASE WHEN @clear_priority::boolean THEN NULL ELSE COALESCE(sqlc.narg('priority_id'), priority_id) END,
    is_active = COALESCE(sqlc.narg('is_active'), is_active),
    sort_order = COALESCE(sqlc.narg('sort_order'), sort_order),
    updated_at = now()
WHERE id = @id
RETURNING *;

-- name: DeleteTopic :execrows
DELETE FROM help_topic WHERE id = $1;

-- name: CountTopicReferences :one
SELECT count(*)::bigint FROM ticket WHERE topic_id = $1;

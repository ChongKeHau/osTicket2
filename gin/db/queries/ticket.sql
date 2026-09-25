-- name: NextTicketNumber :one
SELECT nextval('ticket_number_seq')::bigint;

-- name: CreateTicket :one
-- clock_timestamp() (not now()) so tickets created in quick succession inside
-- one wrapping transaction (as in tests) still get distinct, real-time-ordered
-- created_at/last_message_at values; now() is frozen for the whole transaction.
INSERT INTO ticket (number, subject, status_id, dept_id, topic_id, priority_id,
                    requester_name, requester_email, source, due_at, extra,
                    last_message_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, clock_timestamp(), clock_timestamp(), clock_timestamp())
RETURNING id;

-- name: LockTicket :exec
SELECT id FROM ticket WHERE id = $1 FOR UPDATE;

-- name: GetTicket :one
SELECT t.id, t.number, t.subject,
       t.status_id, s.name AS status_name, s.state AS status_state,
       t.dept_id, d.name AS dept_name,
       t.topic_id, ht.name AS topic_name,
       t.priority_id, p.name AS priority_name,
       t.assigned_staff_id, st.first_name AS assignee_first_name, st.last_name AS assignee_last_name,
       t.requester_name, t.requester_email, t.source, t.is_answered,
       t.due_at, t.closed_at, t.last_message_at, t.last_response_at, t.extra,
       t.created_at, t.updated_at
FROM ticket t
JOIN ticket_status s ON s.id = t.status_id
JOIN department d ON d.id = t.dept_id
JOIN ticket_priority p ON p.id = t.priority_id
LEFT JOIN help_topic ht ON ht.id = t.topic_id
LEFT JOIN staff st ON st.id = t.assigned_staff_id
WHERE t.id = $1;

-- name: ListTickets :many
SELECT t.id, t.number, t.subject,
       t.status_id, s.name AS status_name, s.state AS status_state,
       t.dept_id, d.name AS dept_name,
       t.topic_id, ht.name AS topic_name,
       t.priority_id, p.name AS priority_name,
       t.assigned_staff_id, st.first_name AS assignee_first_name, st.last_name AS assignee_last_name,
       t.requester_name, t.requester_email, t.source, t.is_answered,
       t.due_at, t.closed_at, t.last_message_at, t.last_response_at, t.extra,
       t.created_at, t.updated_at
FROM ticket t
JOIN ticket_status s ON s.id = t.status_id
JOIN department d ON d.id = t.dept_id
JOIN ticket_priority p ON p.id = t.priority_id
LEFT JOIN help_topic ht ON ht.id = t.topic_id
LEFT JOIN staff st ON st.id = t.assigned_staff_id
WHERE (@all_depts::boolean OR t.dept_id = ANY(@dept_ids::bigint[]))
  AND (sqlc.narg('status_id')::bigint IS NULL OR t.status_id = sqlc.narg('status_id'))
  AND (sqlc.narg('state')::text IS NULL OR s.state::text = sqlc.narg('state'))
  AND (sqlc.narg('filter_dept_id')::bigint IS NULL OR t.dept_id = sqlc.narg('filter_dept_id'))
  AND (sqlc.narg('assigned_staff_id')::bigint IS NULL OR t.assigned_staff_id = sqlc.narg('assigned_staff_id'))
  AND (NOT @unassigned::boolean OR t.assigned_staff_id IS NULL)
  AND (@q::text = '' OR to_tsvector('english', t.subject) @@ plainto_tsquery('english', @q))
ORDER BY
  CASE WHEN @sort::text = 'created_at' THEN t.created_at END ASC,
  CASE WHEN @sort::text = '-created_at' THEN t.created_at END DESC,
  CASE WHEN @sort::text = 'last_message_at' THEN t.last_message_at END ASC,
  CASE WHEN @sort::text = '-last_message_at' THEN t.last_message_at END DESC,
  CASE WHEN @sort::text = 'priority' THEN p.urgency END ASC,
  CASE WHEN @sort::text = '-priority' THEN p.urgency END DESC,
  t.id DESC
LIMIT @page_size::int OFFSET @page_offset::int;

-- name: CountTickets :one
SELECT count(*)::bigint
FROM ticket t
JOIN ticket_status s ON s.id = t.status_id
WHERE (@all_depts::boolean OR t.dept_id = ANY(@dept_ids::bigint[]))
  AND (sqlc.narg('status_id')::bigint IS NULL OR t.status_id = sqlc.narg('status_id'))
  AND (sqlc.narg('state')::text IS NULL OR s.state::text = sqlc.narg('state'))
  AND (sqlc.narg('filter_dept_id')::bigint IS NULL OR t.dept_id = sqlc.narg('filter_dept_id'))
  AND (sqlc.narg('assigned_staff_id')::bigint IS NULL OR t.assigned_staff_id = sqlc.narg('assigned_staff_id'))
  AND (NOT @unassigned::boolean OR t.assigned_staff_id IS NULL)
  AND (@q::text = '' OR to_tsvector('english', t.subject) @@ plainto_tsquery('english', @q));

-- name: UpdateTicket :exec
UPDATE ticket
SET subject = COALESCE(sqlc.narg('subject'), subject),
    priority_id = COALESCE(sqlc.narg('priority_id'), priority_id),
    topic_id = CASE WHEN @clear_topic::boolean THEN NULL ELSE COALESCE(sqlc.narg('topic_id'), topic_id) END,
    due_at = CASE WHEN @clear_due_at::boolean THEN NULL ELSE COALESCE(sqlc.narg('due_at'), due_at) END,
    extra = COALESCE(sqlc.narg('extra'), extra),
    requester_name = COALESCE(sqlc.narg('requester_name'), requester_name),
    requester_email = COALESCE(sqlc.narg('requester_email'), requester_email),
    updated_at = now()
WHERE id = @id;

-- name: CreateThreadEntry :one
INSERT INTO thread_entry (ticket_id, type, staff_id, poster, title, body, format, parent_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: CreateTicketEvent :exec
INSERT INTO ticket_event (ticket_id, staff_id, kind, data) VALUES ($1, $2, $3, $4);

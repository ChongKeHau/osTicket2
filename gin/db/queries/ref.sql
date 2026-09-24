-- name: ListPriorities :many
SELECT * FROM ticket_priority ORDER BY urgency;

-- name: GetPriority :one
SELECT * FROM ticket_priority WHERE id = $1;

-- name: DefaultPriority :one
SELECT * FROM ticket_priority ORDER BY (name = 'normal') DESC, urgency LIMIT 1;

-- name: ListStatuses :many
SELECT * FROM ticket_status ORDER BY sort_order;

-- name: GetStatus :one
SELECT * FROM ticket_status WHERE id = $1;

-- name: DefaultStatus :one
SELECT * FROM ticket_status WHERE state = 'open' ORDER BY sort_order LIMIT 1;

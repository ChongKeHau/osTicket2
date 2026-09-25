-- name: GetDepartment :one
SELECT * FROM department WHERE id = $1;

-- name: FirstDepartment :one
SELECT * FROM department ORDER BY id LIMIT 1;

-- name: ListDepartments :many
SELECT * FROM department ORDER BY name;

-- name: CreateDepartment :one
INSERT INTO department (name, is_public, manager_id) VALUES ($1, $2, $3) RETURNING *;

-- name: UpdateDepartment :one
UPDATE department
SET name = COALESCE(sqlc.narg('name'), name),
    is_public = COALESCE(sqlc.narg('is_public'), is_public),
    manager_id = COALESCE(sqlc.narg('manager_id'), manager_id),
    updated_at = now()
WHERE id = @id
RETURNING *;

-- name: DeleteDepartment :execrows
DELETE FROM department WHERE id = $1;

-- name: CountDepartmentReferences :one
SELECT (
  (SELECT count(*) FROM ticket t WHERE t.dept_id = $1) +
  (SELECT count(*) FROM staff s WHERE s.primary_dept_id = $1) +
  (SELECT count(*) FROM staff_department sd WHERE sd.dept_id = $1) +
  (SELECT count(*) FROM help_topic h WHERE h.dept_id = $1)
)::bigint AS refs;

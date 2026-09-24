-- name: GetDepartment :one
SELECT * FROM department WHERE id = $1;

-- name: FirstDepartment :one
SELECT * FROM department ORDER BY id LIMIT 1;

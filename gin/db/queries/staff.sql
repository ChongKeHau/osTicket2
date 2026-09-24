-- name: CreateStaff :one
INSERT INTO staff (username, email, password_hash, first_name, last_name, is_admin, is_active, primary_dept_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetStaff :one
SELECT * FROM staff WHERE id = $1;

-- name: GetStaffByUsername :one
SELECT * FROM staff WHERE username = $1;

-- name: ListStaffDepartmentIDs :many
SELECT dept_id FROM staff_department WHERE staff_id = $1 ORDER BY dept_id;

-- name: AddStaffDepartment :exec
INSERT INTO staff_department (staff_id, dept_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

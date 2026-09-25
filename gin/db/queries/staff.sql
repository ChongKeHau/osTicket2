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

-- name: ListStaff :many
SELECT * FROM staff ORDER BY username;

-- name: UpdateStaff :one
UPDATE staff
SET email = COALESCE(sqlc.narg('email'), email),
    first_name = COALESCE(sqlc.narg('first_name'), first_name),
    last_name = COALESCE(sqlc.narg('last_name'), last_name),
    is_admin = COALESCE(sqlc.narg('is_admin'), is_admin),
    is_active = COALESCE(sqlc.narg('is_active'), is_active),
    primary_dept_id = COALESCE(sqlc.narg('primary_dept_id'), primary_dept_id),
    updated_at = now()
WHERE id = @id
RETURNING *;

-- name: SetStaffPassword :execrows
UPDATE staff SET password_hash = $2, updated_at = now() WHERE id = $1;

-- name: DeleteStaffDepartments :exec
DELETE FROM staff_department WHERE staff_id = $1;

-- name: CountActiveAdmins :one
SELECT count(*)::bigint FROM staff WHERE is_admin AND is_active;

-- name: StaffCanSeeDept :one
SELECT EXISTS (
  SELECT 1 FROM staff s
  LEFT JOIN staff_department sd ON sd.staff_id = s.id AND sd.dept_id = @dept_id
  WHERE s.id = @staff_id AND (s.is_admin OR s.primary_dept_id = @dept_id OR sd.dept_id IS NOT NULL)
)::boolean AS can_see;

-- name: ListActiveStaffForDept :many
SELECT DISTINCT s.* FROM staff s
LEFT JOIN staff_department sd ON sd.staff_id = s.id
WHERE s.is_active AND (s.primary_dept_id = $1 OR sd.dept_id = $1)
ORDER BY s.id;

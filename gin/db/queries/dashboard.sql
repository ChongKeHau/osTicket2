-- name: DashboardSeries :many
SELECT (date_trunc('day', e.created_at AT TIME ZONE 'UTC'))::date AS day,
       count(*) FILTER (WHERE e.kind = 'created')  AS opened,
       count(*) FILTER (WHERE e.kind = 'assigned') AS assigned,
       count(*) FILTER (WHERE e.kind = 'closed')   AS closed,
       count(*) FILTER (WHERE e.kind = 'reopened') AS reopened
FROM ticket_event e
JOIN ticket t ON t.id = e.ticket_id
WHERE e.created_at >= @from_at AND e.created_at < @to_at
  AND e.kind IN ('created', 'assigned', 'closed', 'reopened')
  AND (@all_depts::boolean OR t.dept_id = ANY(@dept_ids::bigint[]))
GROUP BY 1 ORDER BY 1;

-- name: DashboardByDepartment :many
SELECT d.id, d.name,
       count(*) FILTER (WHERE e.kind = 'created')  AS opened,
       count(*) FILTER (WHERE e.kind = 'assigned') AS assigned,
       count(*) FILTER (WHERE e.kind = 'closed')   AS closed,
       count(*) FILTER (WHERE e.kind = 'reopened') AS reopened
FROM ticket_event e
JOIN ticket t ON t.id = e.ticket_id
JOIN department d ON d.id = t.dept_id
WHERE e.created_at >= @from_at AND e.created_at < @to_at
  AND e.kind IN ('created', 'assigned', 'closed', 'reopened')
  AND (@all_depts::boolean OR t.dept_id = ANY(@dept_ids::bigint[]))
GROUP BY d.id, d.name ORDER BY d.name;

-- name: DashboardByTopic :many
SELECT h.id, h.name,
       count(*) FILTER (WHERE e.kind = 'created')  AS opened,
       count(*) FILTER (WHERE e.kind = 'assigned') AS assigned,
       count(*) FILTER (WHERE e.kind = 'closed')   AS closed,
       count(*) FILTER (WHERE e.kind = 'reopened') AS reopened
FROM ticket_event e
JOIN ticket t ON t.id = e.ticket_id
LEFT JOIN help_topic h ON h.id = t.topic_id
WHERE e.created_at >= @from_at AND e.created_at < @to_at
  AND e.kind IN ('created', 'assigned', 'closed', 'reopened')
  AND (@all_depts::boolean OR t.dept_id = ANY(@dept_ids::bigint[]))
GROUP BY h.id, h.name ORDER BY h.name NULLS LAST;

-- name: DashboardByStaff :many
SELECT s.id, s.first_name, s.last_name, s.username,
       count(*) FILTER (WHERE e.kind = 'created')  AS opened,
       count(*) FILTER (WHERE e.kind = 'assigned') AS assigned,
       count(*) FILTER (WHERE e.kind = 'closed')   AS closed,
       count(*) FILTER (WHERE e.kind = 'reopened') AS reopened
FROM ticket_event e
JOIN ticket t ON t.id = e.ticket_id
LEFT JOIN staff s ON s.id = e.staff_id
WHERE e.created_at >= @from_at AND e.created_at < @to_at
  AND e.kind IN ('created', 'assigned', 'closed', 'reopened')
  AND (@all_depts::boolean OR t.dept_id = ANY(@dept_ids::bigint[]))
GROUP BY s.id, s.first_name, s.last_name, s.username ORDER BY s.username NULLS LAST;

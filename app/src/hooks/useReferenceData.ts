import { useQueries } from '@tanstack/react-query'
import { listDepartments, listPriorities, listStaff, listStatuses, listTopics } from '../api/reference'
import type { Department, Staff } from '../api/types'

/** Departments an agent may move a ticket to (the server's CanSeeDept rule), plus the ticket's current one. */
export function visibleDepartments(departments: Department[], auth: { isAdmin: boolean; departmentIds: number[] }, currentId?: number): Department[] {
  return departments.filter((d) => auth.isAdmin || auth.departmentIds.includes(d.id) || d.id === currentId)
}

/** Active agents who can work a ticket in `deptId`: admins, and agents with access to that department. */
export function assignableStaff(staff: Staff[], deptId: number): Staff[] {
  return staff.filter((s) => s.is_active && (s.is_admin || s.primary_dept_id === deptId || s.department_ids.includes(deptId)))
}

export function useReferenceData() {
  const [priorities, statuses, departments, topics, staff] = useQueries({
    queries: [
      { queryKey: ['ref', 'priorities'], queryFn: listPriorities, staleTime: 5 * 60_000 },
      { queryKey: ['ref', 'statuses'], queryFn: listStatuses, staleTime: 5 * 60_000 },
      { queryKey: ['ref', 'departments'], queryFn: listDepartments, staleTime: 5 * 60_000 },
      { queryKey: ['ref', 'topics'], queryFn: listTopics, staleTime: 5 * 60_000 },
      { queryKey: ['ref', 'staff'], queryFn: listStaff, staleTime: 60_000 },
    ],
  })
  return {
    priorities: priorities.data ?? [],
    statuses: statuses.data ?? [],
    departments: departments.data ?? [],
    topics: topics.data ?? [],
    staff: staff.data ?? [],
    isLoading: [priorities, statuses, departments, topics, staff].some((q) => q.isLoading),
    error: [priorities, statuses, departments, topics, staff].find((q) => q.error)?.error ?? null,
  }
}

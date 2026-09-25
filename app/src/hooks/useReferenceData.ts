import { useQueries } from '@tanstack/react-query'
import { listDepartments, listPriorities, listStaff, listStatuses, listTopics } from '../api/reference'

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

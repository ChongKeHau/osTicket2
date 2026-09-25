import { useMutation, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { createDepartment, createStaff, createTopic, deleteDepartment, deleteTopic, setStaffPassword, updateDepartment, updateStaff, updateTopic } from '../api/admin'
import type { CreateStaffInput, DepartmentInput, TopicInput, UpdateStaffInput } from '../api/types'

type Ref = 'departments' | 'topics' | 'staff'

function invalidator(qc: QueryClient, refs: Ref[], tickets: boolean) {
  return async () => {
    await Promise.all([
      ...refs.map((r) => qc.invalidateQueries({ queryKey: ['ref', r] })),
      ...(tickets ? [qc.invalidateQueries({ queryKey: ['tickets'] })] : []),
    ])
  }
}

export function useDepartmentMutations() {
  const qc = useQueryClient()
  // Departments appear in ticket rows, as topic defaults, and as staff memberships.
  const onSettled = invalidator(qc, ['departments', 'topics', 'staff'], true)
  return {
    create: useMutation({ mutationFn: (input: DepartmentInput) => createDepartment(input), onSettled }),
    update: useMutation({ mutationFn: ({ id, input }: { id: number; input: Partial<DepartmentInput> }) => updateDepartment(id, input), onSettled }),
    remove: useMutation({ mutationFn: (id: number) => deleteDepartment(id), onSettled }),
  }
}

export function useTopicMutations() {
  const qc = useQueryClient()
  const onSettled = invalidator(qc, ['topics'], false)
  return {
    create: useMutation({ mutationFn: (input: TopicInput) => createTopic(input), onSettled }),
    update: useMutation({ mutationFn: ({ id, input }: { id: number; input: Partial<TopicInput> }) => updateTopic(id, input), onSettled }),
    remove: useMutation({ mutationFn: (id: number) => deleteTopic(id), onSettled }),
  }
}

export function useStaffMutations() {
  const qc = useQueryClient()
  // Staff names appear as ticket assignees.
  const onSettled = invalidator(qc, ['staff'], true)
  return {
    create: useMutation({ mutationFn: (input: CreateStaffInput) => createStaff(input), onSettled }),
    update: useMutation({ mutationFn: ({ id, input }: { id: number; input: UpdateStaffInput }) => updateStaff(id, input), onSettled }),
    setPassword: useMutation({ mutationFn: ({ id, password }: { id: number; password: string }) => setStaffPassword(id, password) }),
  }
}

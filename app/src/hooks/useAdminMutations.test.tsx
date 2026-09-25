import { QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { makeQueryClient } from '../test/render'
import { server } from '../test/setup'
import { useDepartmentMutations, useStaffMutations, useTopicMutations } from './useAdminMutations'

function setup<T>(hook: () => T) {
  const client = makeQueryClient()
  const spy = vi.spyOn(client, 'invalidateQueries')
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>
  const { result } = renderHook(hook, { wrapper })
  const keys = () => spy.mock.calls.map((c) => (c[0]?.queryKey ?? []).join('.')).sort()
  return { result, keys }
}

test('department update invalidates departments, topics, staff, and tickets', async () => {
  const { result, keys } = setup(() => useDepartmentMutations())
  await result.current.update.mutateAsync({ id: 1, input: { name: 'Help' } })
  await waitFor(() => expect(keys()).toEqual(['ref.departments', 'ref.staff', 'ref.topics', 'tickets']))
})

test('department delete invalidates even when the API returns 404', async () => {
  server.use(http.delete('/api/v1/departments/:id', () => HttpResponse.json({ error: { code: 'not_found', message: 'department not found' } }, { status: 404 })))
  const { result, keys } = setup(() => useDepartmentMutations())
  await expect(result.current.remove.mutateAsync(9)).rejects.toMatchObject({ status: 404 })
  await waitFor(() => expect(keys()).toContain('ref.departments'))
})

test('topic create invalidates topics only', async () => {
  const { result, keys } = setup(() => useTopicMutations())
  await result.current.create.mutateAsync({ name: 'Outages', dept_id: null, priority_id: null, is_active: true, sort_order: 0 })
  await waitFor(() => expect(keys()).toEqual(['ref.topics']))
})

test('staff update invalidates staff and tickets; set password invalidates nothing', async () => {
  const { result, keys } = setup(() => useStaffMutations())
  await result.current.update.mutateAsync({ id: 1, input: { is_active: false } })
  await waitFor(() => expect(keys()).toEqual(['ref.staff', 'tickets']))
  await result.current.setPassword.mutateAsync({ id: 1, password: 'secret123' })
  expect(keys()).toEqual(['ref.staff', 'tickets'])
})

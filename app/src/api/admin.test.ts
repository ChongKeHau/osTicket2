import { http, HttpResponse } from 'msw'
import { referenceFixtures } from '../test/fixtures'
import { server } from '../test/setup'
import { createDepartment, createStaff, createTopic, deleteDepartment, deleteTopic, setStaffPassword, updateDepartment, updateStaff, updateTopic } from './admin'

interface Seen { method: string; path: string; body: unknown }
function capture(): Seen[] {
  const seen: Seen[] = []
  const record = async ({ request }: { request: Request }) => {
    const body = request.method === 'DELETE' ? undefined : await request.json()
    seen.push({ method: request.method, path: new URL(request.url).pathname, body })
  }
  server.use(
    http.post('/api/v1/departments', async (c) => { await record(c); return HttpResponse.json({ ...referenceFixtures.departments[0], id: 9 }, { status: 201 }) }),
    http.patch('/api/v1/departments/:id', async (c) => { await record(c); return HttpResponse.json(referenceFixtures.departments[0]) }),
    http.delete('/api/v1/departments/:id', async (c) => { await record(c); return new HttpResponse(null, { status: 204 }) }),
    http.post('/api/v1/topics', async (c) => { await record(c); return HttpResponse.json({ ...referenceFixtures.topics[0], id: 9 }, { status: 201 }) }),
    http.patch('/api/v1/topics/:id', async (c) => { await record(c); return HttpResponse.json(referenceFixtures.topics[0]) }),
    http.delete('/api/v1/topics/:id', async (c) => { await record(c); return new HttpResponse(null, { status: 204 }) }),
    http.post('/api/v1/staff', async (c) => { await record(c); return HttpResponse.json({ ...referenceFixtures.staff[0], id: 9 }, { status: 201 }) }),
    http.patch('/api/v1/staff/:id', async (c) => { await record(c); return HttpResponse.json(referenceFixtures.staff[0]) }),
    http.post('/api/v1/staff/:id/password', async (c) => { await record(c); return new HttpResponse(null, { status: 204 }) }),
  )
  return seen
}

test('department calls hit the admin endpoints with JSON bodies', async () => {
  const seen = capture()
  const created = await createDepartment({ name: 'Sales', is_public: false, manager_id: 1 })
  expect(created.id).toBe(9)
  await updateDepartment(2, { name: 'Billing & Payments' })
  await expect(deleteDepartment(2)).resolves.toBeUndefined()
  expect(seen).toEqual([
    { method: 'POST', path: '/api/v1/departments', body: { name: 'Sales', is_public: false, manager_id: 1 } },
    { method: 'PATCH', path: '/api/v1/departments/2', body: { name: 'Billing & Payments' } },
    { method: 'DELETE', path: '/api/v1/departments/2', body: undefined },
  ])
})

test('topic calls hit the admin endpoints', async () => {
  const seen = capture()
  await createTopic({ name: 'Outages', dept_id: 1, priority_id: 3, is_active: true, sort_order: 5 })
  await updateTopic(1, { is_active: false })
  await deleteTopic(1)
  expect(seen.map((s) => `${s.method} ${s.path}`)).toEqual(['POST /api/v1/topics', 'PATCH /api/v1/topics/1', 'DELETE /api/v1/topics/1'])
  expect(seen[0]!.body).toEqual({ name: 'Outages', dept_id: 1, priority_id: 3, is_active: true, sort_order: 5 })
})

test('staff calls hit the admin endpoints; set password posts the password only', async () => {
  const seen = capture()
  await createStaff({ username: 'new', email: 'new@example.test', password: 'secret123', first_name: 'New', last_name: 'Person', is_admin: false, primary_dept_id: 1, department_ids: [1, 2] })
  await updateStaff(2, { is_active: false })
  await expect(setStaffPassword(2, 'another123')).resolves.toBeUndefined()
  expect(seen.map((s) => `${s.method} ${s.path}`)).toEqual(['POST /api/v1/staff', 'PATCH /api/v1/staff/2', 'POST /api/v1/staff/2/password'])
  expect(seen[1]!.body).toEqual({ is_active: false })
  expect(seen[2]!.body).toEqual({ password: 'another123' })
})

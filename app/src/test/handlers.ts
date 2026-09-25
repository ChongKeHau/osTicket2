import { http, HttpResponse } from 'msw'
import { entryFixtures, eventFixtures, referenceFixtures, sessionFixture, staffProfileFixture, ticketFixture } from './fixtures'

const unauthorized = () => HttpResponse.json({ error: { code: 'unauthorized', message: 'authentication required' } }, { status: 401 })

export const handlers = [
  http.post('/api/v1/auth/login', async ({ request }) => {
    const body = (await request.json()) as { username: string; password: string }
    if (body.username === 'agent' && body.password === 'password1') return HttpResponse.json(sessionFixture)
    return unauthorized()
  }),
  http.post('/api/v1/auth/refresh', async ({ request }) => {
    const body = (await request.json()) as { refresh_token: string }
    if (body.refresh_token.startsWith('refresh')) {
      return HttpResponse.json({ ...sessionFixture, access_token: 'access-2', refresh_token: 'refresh-2' })
    }
    return unauthorized()
  }),
  http.post('/api/v1/auth/logout', () => new HttpResponse(null, { status: 204 })),
  http.get('/api/v1/me', () => HttpResponse.json(staffProfileFixture)),
  http.get('/api/v1/priorities', () => HttpResponse.json({ items: referenceFixtures.priorities })),
  http.get('/api/v1/statuses', () => HttpResponse.json({ items: referenceFixtures.statuses })),
  http.get('/api/v1/departments', () => HttpResponse.json({ items: referenceFixtures.departments })),
  http.get('/api/v1/topics', () => HttpResponse.json({ items: referenceFixtures.topics })),
  http.get('/api/v1/staff', () => HttpResponse.json({ items: referenceFixtures.staff })),
  http.get('/api/v1/tickets', () => HttpResponse.json({ items: [ticketFixture], page: 1, page_size: 25, total: 1 })),
  http.get('/api/v1/tickets/:id', ({ params }) =>
    params.id === '7' ? HttpResponse.json(ticketFixture)
      : HttpResponse.json({ error: { code: 'not_found', message: 'ticket not found' } }, { status: 404 })),
  http.get('/api/v1/tickets/:id/thread', () => HttpResponse.json({ items: entryFixtures, next_after: null })),
  http.get('/api/v1/tickets/:id/events', () => HttpResponse.json({ items: eventFixtures })),
  http.post('/api/v1/tickets', () => HttpResponse.json({ ...ticketFixture, id: 8, number: '000008' }, { status: 201 })),
  http.patch('/api/v1/tickets/:id', () => HttpResponse.json(ticketFixture)),
  http.post('/api/v1/tickets/:id/reply', () => HttpResponse.json({ ...entryFixtures[1], id: 9 }, { status: 201 })),
  http.post('/api/v1/tickets/:id/notes', () => HttpResponse.json({ ...entryFixtures[1], id: 10, type: 'note' }, { status: 201 })),
  http.post('/api/v1/tickets/:id/status', () => HttpResponse.json({ ...ticketFixture, status: { id: 3, name: 'Closed' }, state: 'closed' })),
  http.post('/api/v1/tickets/:id/assign', () => HttpResponse.json({ ...ticketFixture, assignee: { id: 1, name: 'Ann Agent' } })),
  http.post('/api/v1/tickets/:id/transfer', () => HttpResponse.json({ ...ticketFixture, department: { id: 2, name: 'Billing' } })),
  http.post('/api/v1/files', () => HttpResponse.json({ id: 42, name: 'a.txt', mime: 'text/plain', size: 3 }, { status: 201 })),
  http.get('/api/v1/files/:id', () => new HttpResponse('hello', { headers: { 'Content-Type': 'text/plain' } })),
  http.post('/api/v1/departments', async ({ request }) => HttpResponse.json({ id: 9, is_public: true, manager_id: null, ...(await request.json() as object) }, { status: 201 })),
  http.patch('/api/v1/departments/:id', async ({ request, params }) => HttpResponse.json({ ...referenceFixtures.departments[0], ...(await request.json() as object), id: Number(params.id) })),
  http.delete('/api/v1/departments/:id', () => new HttpResponse(null, { status: 204 })),
  http.post('/api/v1/topics', async ({ request }) => HttpResponse.json({ id: 9, dept_id: null, priority_id: null, is_active: true, sort_order: 0, ...(await request.json() as object) }, { status: 201 })),
  http.patch('/api/v1/topics/:id', async ({ request, params }) => HttpResponse.json({ ...referenceFixtures.topics[0], ...(await request.json() as object), id: Number(params.id) })),
  http.delete('/api/v1/topics/:id', () => new HttpResponse(null, { status: 204 })),
  http.post('/api/v1/staff', async ({ request }) => {
    const body = await request.json() as Record<string, unknown>
    delete body.password
    return HttpResponse.json({ id: 9, is_active: true, department_ids: [], ...body }, { status: 201 })
  }),
  http.patch('/api/v1/staff/:id', async ({ request, params }) => HttpResponse.json({ ...referenceFixtures.staff[0], ...(await request.json() as object), id: Number(params.id) })),
  http.post('/api/v1/staff/:id/password', () => new HttpResponse(null, { status: 204 })),
]

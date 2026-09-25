import { http, HttpResponse } from 'msw'
import { server } from '../test/setup'
import { sessionFixture } from '../test/fixtures'
import { login, logout, me } from './auth'
import { REFRESH_KEY, tokens } from './client'
import { downloadFile, uploadFile } from './files'
import { listStaff, listStatuses } from './reference'
import { assign, listTickets, reply } from './tickets'

beforeEach(() => tokens.clear())

test('login stores tokens; logout posts refresh token and clears', async () => {
  const s = await login('agent', 'password1')
  expect(s.staff.username).toBe('agent')
  expect(tokens.access).toBe('access-1')
  expect(localStorage.getItem(REFRESH_KEY)).toBe('refresh-1')
  let sent = ''
  server.use(http.post('/api/v1/auth/logout', async ({ request }) => { sent = ((await request.json()) as { refresh_token: string }).refresh_token; return new HttpResponse(null, { status: 204 }) }))
  await logout()
  expect(sent).toBe('refresh-1')
  expect(tokens.access).toBeNull()
  expect(localStorage.getItem(REFRESH_KEY)).toBeNull()
})

test('logout clears even when the API call fails', async () => {
  tokens.setSession(sessionFixture)
  server.use(http.post('/api/v1/auth/logout', () => HttpResponse.json({ error: { code: 'internal', message: 'x' } }, { status: 500 })))
  await logout()
  expect(tokens.access).toBeNull()
})

test('me returns the profile', async () => {
  tokens.setSession(sessionFixture)
  expect((await me()).department_ids).toEqual([1])
})

test('listTickets sends filters as query params', async () => {
  let url = ''
  server.use(http.get('/api/v1/tickets', ({ request }) => { url = request.url; return HttpResponse.json({ items: [], page: 2, page_size: 10, total: 0 }) }))
  tokens.setSession(sessionFixture)
  await listTickets({ state: 'open', assigned_to: 'me', q: 'printer', sort: '-priority', page: 2, page_size: 10 })
  const p = new URL(url).searchParams
  expect(p.get('state')).toBe('open'); expect(p.get('assigned_to')).toBe('me'); expect(p.get('q')).toBe('printer')
  expect(p.get('sort')).toBe('-priority'); expect(p.get('page')).toBe('2'); expect(p.get('page_size')).toBe('10')
})

test('reply and assign post the right bodies', async () => {
  const bodies: unknown[] = []
  server.use(
    http.post('/api/v1/tickets/7/reply', async ({ request }) => { bodies.push(await request.json()); return HttpResponse.json({ id: 1 }, { status: 201 }) }),
    http.post('/api/v1/tickets/7/assign', async ({ request }) => { bodies.push(await request.json()); return HttpResponse.json({ id: 7 }) }),
  )
  tokens.setSession(sessionFixture)
  await reply(7, { body: 'hi', format: 'text', status_id: 3, file_ids: [42] })
  await assign(7, null)
  expect(bodies[0]).toEqual({ body: 'hi', format: 'text', status_id: 3, file_ids: [42] })
  expect(bodies[1]).toEqual({ staff_id: null })
})

test('reference lists unwrap items', async () => {
  tokens.setSession(sessionFixture)
  expect((await listStatuses()).map((s) => s.state)).toEqual(['open', 'resolved', 'closed'])
  expect((await listStaff()).length).toBe(3)
})

test('uploadFile posts multipart and downloadFile fetches with auth', async () => {
  let contentType = ''
  let auth = ''
  server.use(
    http.post('/api/v1/files', ({ request }) => { contentType = request.headers.get('content-type') ?? ''; return HttpResponse.json({ id: 42, name: 'a.txt', mime: 'text/plain', size: 3 }, { status: 201 }) }),
    http.get('/api/v1/files/42', ({ request }) => { auth = request.headers.get('authorization') ?? ''; return new HttpResponse('abc', { headers: { 'Content-Type': 'text/plain' } }) }),
  )
  tokens.setSession(sessionFixture)
  const info = await uploadFile(new File(['abc'], 'a.txt', { type: 'text/plain' }))
  expect(info.id).toBe(42)
  expect(contentType).toMatch(/^multipart\/form-data/)
  const createObjectURL = vi.fn(() => 'blob:x')
  const revokeObjectURL = vi.fn()
  Object.assign(URL, { createObjectURL, revokeObjectURL })
  const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  await downloadFile(42, 'a.txt')
  expect(auth).toBe('Bearer access-1')
  expect(createObjectURL).toHaveBeenCalledTimes(1)
  expect(click).toHaveBeenCalledTimes(1)
  expect(revokeObjectURL).toHaveBeenCalledWith('blob:x')
  click.mockRestore()
})

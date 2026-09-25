import { http, HttpResponse } from 'msw'
import { server } from '../test/setup'
import { sessionFixture } from '../test/fixtures'
import { login, logout, me } from './auth'
import { REFRESH_KEY, refreshSession, tokens } from './client'
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

test('logout with no access token refreshes first and revokes the rotated token', async () => {
  const calls: string[] = []
  let sent = ''
  let auth = ''
  server.use(
    http.post('/api/v1/auth/refresh', () => { calls.push('refresh'); return HttpResponse.json({ ...sessionFixture, access_token: 'access-2', refresh_token: 'refresh-2' }) }),
    http.post('/api/v1/auth/logout', async ({ request }) => {
      calls.push('logout')
      auth = request.headers.get('authorization') ?? ''
      sent = ((await request.json()) as { refresh_token: string }).refresh_token
      return new HttpResponse(null, { status: 204 })
    }),
  )
  localStorage.setItem(REFRESH_KEY, 'refresh-1') // e.g. page idle past the access-token lifetime, then reloaded
  await logout()
  expect(calls).toEqual(['refresh', 'logout'])
  expect(auth).toBe('Bearer access-2')
  expect(sent).toBe('refresh-2')
  expect(localStorage.getItem(REFRESH_KEY)).toBeNull()
})

test('logout that gets 401 refreshes once and posts again', async () => {
  const sent: string[] = []
  server.use(
    http.post('/api/v1/auth/logout', async ({ request }) => {
      sent.push(((await request.json()) as { refresh_token: string }).refresh_token)
      if (request.headers.get('authorization') !== 'Bearer access-2') return HttpResponse.json({ error: { code: 'unauthorized', message: 'x' } }, { status: 401 })
      return new HttpResponse(null, { status: 204 })
    }),
  )
  tokens.setSession(sessionFixture) // access-1 has expired server-side
  await logout()
  expect(sent).toEqual(['refresh-1', 'refresh-2'])
  expect(tokens.access).toBeNull()
  expect(localStorage.getItem(REFRESH_KEY)).toBeNull()
})

test('logout during an in-flight refresh waits for it and ends with storage empty', async () => {
  let release!: () => void
  const gate = new Promise<void>((r) => { release = r })
  let sent = ''
  server.use(
    http.post('/api/v1/auth/refresh', async () => { await gate; return HttpResponse.json({ ...sessionFixture, access_token: 'access-2', refresh_token: 'refresh-2' }) }),
    http.post('/api/v1/auth/logout', async ({ request }) => { sent = ((await request.json()) as { refresh_token: string }).refresh_token; return new HttpResponse(null, { status: 204 }) }),
  )
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  const inFlight = refreshSession()
  const out = logout()
  release()
  await Promise.all([inFlight, out])
  expect(sent).toBe('refresh-2')
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
  const setTimeoutSpy = vi.spyOn(globalThis, 'setTimeout')
  await downloadFile(42, 'a.txt')
  expect(auth).toBe('Bearer access-1')
  expect(createObjectURL).toHaveBeenCalledTimes(1)
  expect(click).toHaveBeenCalledTimes(1)
  // Revocation is deferred so the browser can start the download first.
  expect(revokeObjectURL).not.toHaveBeenCalled()
  const deferred = setTimeoutSpy.mock.calls.find(([, ms]) => ms === 1000)
  expect(deferred).toBeDefined()
  ;(deferred?.[0] as () => void)()
  expect(revokeObjectURL).toHaveBeenCalledWith('blob:x')
  setTimeoutSpy.mockRestore()
  click.mockRestore()
})

import { http, HttpResponse } from 'msw'
import { server } from '../test/setup'
import { sessionFixture } from '../test/fixtures'
import { ApiError, REFRESH_KEY, refreshSession, request, tokens } from './client'

beforeEach(() => tokens.clear())

test('attaches bearer header and parses JSON', async () => {
  let auth = ''
  server.use(http.get('/api/v1/ping', ({ request: req }) => { auth = req.headers.get('authorization') ?? ''; return HttpResponse.json({ ok: true }) }))
  tokens.setSession(sessionFixture)
  const out = await request<{ ok: boolean }>('GET', '/ping')
  expect(out.ok).toBe(true)
  expect(auth).toBe('Bearer access-1')
})

test('maps the error envelope to ApiError', async () => {
  server.use(http.post('/api/v1/tickets', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { subject: 'required' } } }, { status: 400 })))
  await expect(request('POST', '/tickets', { body: {} })).rejects.toMatchObject({ status: 400, code: 'validation_failed', fields: { subject: 'required' } })
  const err = await request('POST', '/tickets', { body: {} }).catch((e: unknown) => e)
  expect(err).toBeInstanceOf(ApiError)
})

test('non-JSON failure becomes a network ApiError', async () => {
  server.use(http.get('/api/v1/boom', () => new HttpResponse('<html>bad gateway</html>', { status: 502 })))
  await expect(request('GET', '/boom')).rejects.toMatchObject({ status: 502, code: 'network' })
})

test('appends query params and drops empty ones', async () => {
  let url = ''
  server.use(http.get('/api/v1/tickets', ({ request: req }) => { url = req.url; return HttpResponse.json({ items: [] }) }))
  await request('GET', '/tickets', { query: { page: 2, q: '', state: undefined, sort: '-priority' } })
  expect(new URL(url).search).toBe('?page=2&sort=-priority')
})

test('refreshes once on 401 and retries; concurrent 401s share one refresh', async () => {
  let refreshCalls = 0
  let pingCalls = 0
  server.use(
    http.post('/api/v1/auth/refresh', () => { refreshCalls++; return HttpResponse.json({ ...sessionFixture, access_token: 'access-2', refresh_token: 'refresh-2' }) }),
    http.get('/api/v1/ping', ({ request: req }) => {
      pingCalls++
      if (req.headers.get('authorization') !== 'Bearer access-2') {
        return HttpResponse.json({ error: { code: 'unauthorized', message: 'authentication required' } }, { status: 401 })
      }
      return HttpResponse.json({ ok: true })
    }),
  )
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  const [a, b] = await Promise.all([request<{ ok: boolean }>('GET', '/ping'), request<{ ok: boolean }>('GET', '/ping')])
  expect(a.ok && b.ok).toBe(true)
  expect(refreshCalls).toBe(1)
  expect(pingCalls).toBe(4)
  expect(localStorage.getItem(REFRESH_KEY)).toBe('refresh-2')
  expect(tokens.access).toBe('access-2')
})

test('failed refresh clears the session and notifies', async () => {
  const lost = vi.fn()
  tokens.setOnSessionLost(lost)
  server.use(
    http.post('/api/v1/auth/refresh', () => HttpResponse.json({ error: { code: 'unauthorized', message: 'x' } }, { status: 401 })),
    http.get('/api/v1/ping', () => HttpResponse.json({ error: { code: 'unauthorized', message: 'x' } }, { status: 401 })),
  )
  localStorage.setItem(REFRESH_KEY, 'refresh-stale')
  await expect(request('GET', '/ping')).rejects.toMatchObject({ status: 401 })
  expect(lost).toHaveBeenCalledTimes(1)
  expect(localStorage.getItem(REFRESH_KEY)).toBeNull()
  expect(await refreshSession()).toBe(false)
  tokens.setOnSessionLost(null)
})

const unauthorized = () => HttpResponse.json({ error: { code: 'unauthorized', message: 'x' } }, { status: 401 })
const issued = (n: number) => HttpResponse.json({ ...sessionFixture, access_token: `access-${n}`, refresh_token: `refresh-${n}` })

test('another tab rotating the shared token mid-refresh does not sign this tab out', async () => {
  // Server: each refresh token is single-use and rotates to the next number.
  const valid = new Set(['refresh-1'])
  let next = 2
  let releaseThisTab!: () => void
  const otherTabDone = new Promise<void>((r) => { releaseThisTab = r })
  server.use(http.post('/api/v1/auth/refresh', async ({ request: req }) => {
    const { refresh_token } = (await req.json()) as { refresh_token: string }
    if (req.headers.get('x-tab') !== 'other') await otherTabDone // the other tab's request wins the race
    if (!valid.delete(refresh_token)) return unauthorized()
    const n = next++
    valid.add(`refresh-${n}`)
    return issued(n)
  }))
  const lost = vi.fn()
  tokens.setOnSessionLost(lost)
  localStorage.setItem(REFRESH_KEY, 'refresh-1')

  const thisTab = refreshSession() // sends refresh-1
  // The other tab (same localStorage) refreshes with refresh-1 too and stores its rotated token.
  const res = await fetch('/api/v1/auth/refresh', {
    method: 'POST', headers: { 'Content-Type': 'application/json', 'x-tab': 'other' }, body: JSON.stringify({ refresh_token: 'refresh-1' }),
  })
  localStorage.setItem(REFRESH_KEY, ((await res.json()) as { refresh_token: string }).refresh_token)
  releaseThisTab()

  expect(await thisTab).toBe(true) // got 401 for refresh-1, saw refresh-2 stored, retried with it
  expect(lost).not.toHaveBeenCalled()
  expect(localStorage.getItem(REFRESH_KEY)).toBe('refresh-3')
  expect(tokens.access).toBe('access-3')
  tokens.setOnSessionLost(null)
})

test('a 401 with no stored refresh token but a held access token ends the session once', async () => {
  const lost = vi.fn()
  tokens.setOnSessionLost(lost)
  let refreshCalls = 0
  server.use(
    http.post('/api/v1/auth/refresh', () => { refreshCalls++; return issued(2) }),
    http.get('/api/v1/ping', unauthorized),
  )
  tokens.setSession(sessionFixture)
  localStorage.removeItem(REFRESH_KEY) // e.g. another tab cleared it
  await expect(request('GET', '/ping')).rejects.toMatchObject({ status: 401 })
  expect(lost).toHaveBeenCalledTimes(1)
  expect(tokens.access).toBeNull()
  expect(refreshCalls).toBe(0)
  await expect(request('GET', '/ping')).rejects.toMatchObject({ status: 401 })
  expect(lost).toHaveBeenCalledTimes(1)
  tokens.setOnSessionLost(null)
})

test('a 5xx or network failure on refresh keeps the token and does not notify', async () => {
  const lost = vi.fn()
  tokens.setOnSessionLost(lost)
  server.use(
    http.post('/api/v1/auth/refresh', () => new HttpResponse('bad gateway', { status: 502 })),
    http.get('/api/v1/ping', unauthorized),
  )
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  expect(await refreshSession()).toBe(false)
  await expect(request('GET', '/ping')).rejects.toMatchObject({ status: 401 })
  server.use(http.post('/api/v1/auth/refresh', () => HttpResponse.error()))
  expect(await refreshSession()).toBe(false)
  expect(lost).not.toHaveBeenCalled()
  expect(localStorage.getItem(REFRESH_KEY)).toBe('refresh-1')
  tokens.setOnSessionLost(null)
})

test('a refresh that resolves after tokens.clear() does not restore the session', async () => {
  let release!: () => void
  const gate = new Promise<void>((r) => { release = r })
  server.use(http.post('/api/v1/auth/refresh', async () => { await gate; return issued(2) }))
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  const pending = refreshSession()
  tokens.clear()
  release()
  expect(await pending).toBe(false)
  expect(tokens.access).toBeNull()
  expect(localStorage.getItem(REFRESH_KEY)).toBeNull()
})

test('does not try to refresh for /auth/* paths', async () => {
  let refreshCalls = 0
  server.use(http.post('/api/v1/auth/refresh', () => { refreshCalls++; return HttpResponse.json(sessionFixture) }))
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  await expect(request('POST', '/auth/login', { body: { username: 'x', password: 'y' } })).rejects.toMatchObject({ status: 401 })
  expect(refreshCalls).toBe(0)
})

test('returns undefined for 204', async () => {
  server.use(http.post('/api/v1/auth/logout', () => new HttpResponse(null, { status: 204 })))
  await expect(request('POST', '/auth/logout', { body: { refresh_token: 'r' } })).resolves.toBeUndefined()
})

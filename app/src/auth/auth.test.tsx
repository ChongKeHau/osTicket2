import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../App'
import { REFRESH_KEY, tokens } from '../api/client'
import { sessionFixture } from '../test/fixtures'
import { renderWithProviders } from '../test/render'
import { server } from '../test/setup'

beforeEach(() => tokens.clear())

test('anonymous visit to /tickets redirects to /login and back after login', async () => {
  renderWithProviders(<App />, { route: '/tickets' })
  expect(await screen.findByRole('heading', { name: 'Ticket Desk' })).toBeInTheDocument()
  await userEvent.type(screen.getByLabelText(/username/i), 'agent')
  await userEvent.type(screen.getByLabelText(/password/i), 'password1')
  await userEvent.click(screen.getByRole('button', { name: /sign in/i }))
  expect(await screen.findByText(/Ann Agent/)).toBeInTheDocument()
  expect(localStorage.getItem(REFRESH_KEY)).toBe('refresh-1')
  expect(screen.queryByRole('heading', { name: 'Ticket Desk' })).not.toBeInTheDocument()
})

test('wrong password shows an error', async () => {
  renderWithProviders(<App />, { route: '/login' })
  await userEvent.type(await screen.findByLabelText(/username/i), 'agent')
  await userEvent.type(screen.getByLabelText(/password/i), 'nope')
  await userEvent.click(screen.getByRole('button', { name: /sign in/i }))
  expect(await screen.findByText(/invalid username or password/i)).toBeInTheDocument()
})

test('reload with a stored refresh token restores the session', async () => {
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  renderWithProviders(<App />, { route: '/tickets' })
  expect(await screen.findByText(/Ann Agent/)).toBeInTheDocument()
  expect(tokens.access).toBe('access-2')
})

test('stale refresh token on reload lands on login with a notice', async () => {
  localStorage.setItem(REFRESH_KEY, 'stale')
  renderWithProviders(<App />, { route: '/tickets' })
  expect(await screen.findByRole('heading', { name: 'Ticket Desk' })).toBeInTheDocument()
  expect(screen.getByText(/session expired/i)).toBeInTheDocument()
  expect(localStorage.getItem(REFRESH_KEY)).toBeNull()
})

test('a 502 on refresh at startup keeps the token and offers Retry', async () => {
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  server.use(http.post('/api/v1/auth/refresh', () => new HttpResponse('bad gateway', { status: 502 })))
  renderWithProviders(<App />, { route: '/tickets' })
  expect(await screen.findByRole('alert')).toHaveTextContent(/could not reach the server/i)
  expect(localStorage.getItem(REFRESH_KEY)).toBe('refresh-1')
  expect(screen.queryByRole('heading', { name: 'Ticket Desk' })).not.toBeInTheDocument()
  server.resetHandlers()
  await userEvent.click(screen.getByRole('button', { name: /retry/i }))
  expect(await screen.findByText(/Ann Agent/)).toBeInTheDocument()
  expect(await screen.findByText('Printer on fire')).toBeInTheDocument()
})

test('a 500 from /me at startup offers Retry; a 401 from /me shows the expired notice', async () => {
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  server.use(http.get('/api/v1/me', () => HttpResponse.json({ error: { code: 'internal', message: 'internal error' } }, { status: 500 })))
  const { unmount } = renderWithProviders(<App />, { route: '/tickets' })
  expect(await screen.findByRole('button', { name: /retry/i })).toBeInTheDocument()
  expect(localStorage.getItem(REFRESH_KEY)).toBe('refresh-2')
  unmount()
  let refreshCalls = 0
  server.use(
    http.get('/api/v1/me', () => HttpResponse.json({ error: { code: 'unauthorized', message: 'x' } }, { status: 401 })),
    // Startup refresh succeeds; the retry refresh triggered by /me's 401 is rejected.
    http.post('/api/v1/auth/refresh', () => (++refreshCalls === 1
      ? HttpResponse.json({ ...sessionFixture, access_token: 'access-3', refresh_token: 'refresh-3' })
      : HttpResponse.json({ error: { code: 'unauthorized', message: 'x' } }, { status: 401 }))),
  )
  renderWithProviders(<App />, { route: '/tickets' })
  expect(await screen.findByRole('heading', { name: 'Ticket Desk' })).toBeInTheDocument()
  expect(screen.getByText(/session expired/i)).toBeInTheDocument()
})

test('logout clears storage and returns to login', async () => {
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  renderWithProviders(<App />, { route: '/tickets' })
  await screen.findByText(/Ann Agent/)
  await userEvent.click(screen.getByRole('button', { name: 'Log Out' }))
  expect(await screen.findByRole('heading', { name: 'Ticket Desk' })).toBeInTheDocument()
  expect(localStorage.getItem(REFRESH_KEY)).toBeNull()
})

test('mid-session refresh failure routes to login with the notice', async () => {
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  const { client } = renderWithProviders(<App />, { route: '/tickets' })
  await screen.findByText(/Ann Agent/)
  await screen.findByText('Printer on fire')
  server.use(
    http.get('/api/v1/tickets', () => HttpResponse.json({ error: { code: 'unauthorized', message: 'x' } }, { status: 401 })),
    http.get('/api/v1/topics', () => HttpResponse.json({ error: { code: 'unauthorized', message: 'x' } }, { status: 401 })),
    http.post('/api/v1/auth/refresh', () => HttpResponse.json({ error: { code: 'unauthorized', message: 'x' } }, { status: 401 })),
  )
  // NewTicketPage's reference-data fetch (including /topics) would otherwise be served from the
  // FilterBar's already-fresh cache (useReferenceData has a 5-minute staleTime), never issuing a
  // new request. Clear the cache so navigating to /tickets/new triggers a real GET /topics that
  // hits the 401 override above.
  client.clear()
  await userEvent.click(screen.getByRole('link', { name: /new ticket/i }))
  await waitFor(() => expect(screen.getByRole('heading', { name: 'Ticket Desk' })).toBeInTheDocument())
  expect(screen.getByText(/session expired/i)).toBeInTheDocument()
})

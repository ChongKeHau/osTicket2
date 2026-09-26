import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { REFRESH_KEY } from '../api/client'
import { exchange } from '../api/portal'
import { PORTAL_REFRESH_KEY, portal } from '../api/portalClient'
import { portalFixtures } from '../test/portal'
import { renderWithProviders } from '../test/render'
import { server } from '../test/setup'
import { PortalAuthProvider, usePortalAuth } from './PortalAuthContext'

beforeEach(() => portal.tokens.clear())

function Probe() {
  const a = usePortalAuth()
  return (
    <div>
      <p>status:{a.status}</p>
      <p>user:{a.user?.email ?? '-'}</p>
      <p>ticket:{a.ticketId ?? '-'}</p>
      <p>guest:{String(a.isGuest)}</p>
      <p>notice:{a.notice ?? '-'}</p>
      <button type="button" onClick={() => a.adopt(portalFixtures.guestSession)}>adopt</button>
      <button type="button" onClick={() => void a.login('pat@example.test', 'secret123')}>login</button>
      <button type="button" onClick={() => void a.logout()}>logout</button>
    </div>
  )
}

const renderProbe = () => renderWithProviders(<PortalAuthProvider><Probe /></PortalAuthProvider>)

it('restores an account session from the portal key and never touches the staff key', async () => {
  const calls: string[] = []
  server.use(
    http.post('/api/v1/auth/refresh', () => { calls.push('staff-refresh'); return HttpResponse.json({}, { status: 500 }) }),
    http.get('/api/v1/me', () => { calls.push('staff-me'); return HttpResponse.json({}, { status: 500 }) }),
  )
  server.events.on('request:start', ({ request }) => { calls.push(new URL(request.url).pathname) })
  localStorage.setItem(PORTAL_REFRESH_KEY, 'prefresh-1')
  localStorage.setItem(REFRESH_KEY, 'refresh-staff')
  renderProbe()
  expect(await screen.findByText('status:authenticated')).toBeInTheDocument()
  server.events.removeAllListeners()
  expect(screen.getByText('user:pat@example.test')).toBeInTheDocument()
  expect(screen.getByText('guest:false')).toBeInTheDocument()
  expect(calls).toEqual(['/api/v1/portal/auth/refresh', '/api/v1/portal/me'])
  expect(localStorage.getItem(PORTAL_REFRESH_KEY)).toBe('prefresh-2')
  expect(localStorage.getItem(REFRESH_KEY)).toBe('refresh-staff')
})

it('restores a guest session scoped to its ticket', async () => {
  localStorage.setItem(PORTAL_REFRESH_KEY, 'prefresh-guest-1')
  renderProbe()
  expect(await screen.findByText('status:authenticated')).toBeInTheDocument()
  expect(screen.getByText('ticket:7')).toBeInTheDocument()
  expect(screen.getByText('guest:true')).toBeInTheDocument()
})

it('is anonymous without a stored token and shows the expiry notice when the token is rejected', async () => {
  const { unmount } = renderProbe()
  expect(await screen.findByText('status:anonymous')).toBeInTheDocument()
  expect(screen.getByText('notice:-')).toBeInTheDocument()
  unmount()
  localStorage.setItem(PORTAL_REFRESH_KEY, 'stale')
  renderProbe()
  expect(await screen.findByText('notice:Your session expired. Please sign in again.')).toBeInTheDocument()
  expect(localStorage.getItem(PORTAL_REFRESH_KEY)).toBeNull()
})

it('ends a restored session cleanly when /me is forbidden to it', async () => {
  server.use(http.get('/api/v1/portal/me', () => HttpResponse.json({ error: { code: 'forbidden', message: 'no' } }, { status: 403 })))
  localStorage.setItem(PORTAL_REFRESH_KEY, 'prefresh-1')
  renderProbe()
  expect(await screen.findByText('status:anonymous')).toBeInTheDocument()
  expect(screen.getByText('notice:-')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Retry' })).not.toBeInTheDocument()
  expect(localStorage.getItem(PORTAL_REFRESH_KEY)).toBeNull()
  expect(portal.tokens.access).toBeNull()
})

it('a reset session from a token exchange is not persisted', async () => {
  const s = await exchange('good-reset')
  expect(s.kind).toBe('reset')
  expect(portal.tokens.access).toBe('paccess-reset')
  expect(localStorage.getItem(PORTAL_REFRESH_KEY)).toBeNull()
})

it('adopt stores the session and sets the user and guest ticket', async () => {
  renderProbe()
  await screen.findByText('status:anonymous')
  await userEvent.click(screen.getByRole('button', { name: 'adopt' }))
  expect(screen.getByText('status:authenticated')).toBeInTheDocument()
  expect(screen.getByText('ticket:7')).toBeInTheDocument()
  expect(screen.getByText('guest:true')).toBeInTheDocument()
  expect(localStorage.getItem(PORTAL_REFRESH_KEY)).toBe('prefresh-guest-1')
  expect(portal.tokens.access).toBe('paccess-guest-1')
  expect(localStorage.getItem(REFRESH_KEY)).toBeNull()
})

it('signs in with a password and signs out', async () => {
  renderProbe()
  await screen.findByText('status:anonymous')
  await userEvent.click(screen.getByRole('button', { name: 'login' }))
  expect(await screen.findByText('status:authenticated')).toBeInTheDocument()
  expect(screen.getByText('user:pat@example.test')).toBeInTheDocument()
  expect(screen.getByText('guest:false')).toBeInTheDocument()
  expect(localStorage.getItem(PORTAL_REFRESH_KEY)).toBe('prefresh-1')
  await userEvent.click(screen.getByRole('button', { name: 'logout' }))
  expect(await screen.findByText('status:anonymous')).toBeInTheDocument()
  expect(localStorage.getItem(PORTAL_REFRESH_KEY)).toBeNull()
  expect(portal.tokens.access).toBeNull()
})

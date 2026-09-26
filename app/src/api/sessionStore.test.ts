import { http, HttpResponse } from 'msw'
import { server } from '../test/setup'
import { createSessionStore } from './sessionStore'

const staffKey = 'ticket.refresh_token'
const portalKey = 'ticket.portal_refresh_token'

it('keeps two stores on separate storage keys and refresh paths', async () => {
  const calls: string[] = []
  server.use(
    http.post('/api/v1/auth/refresh', () => { calls.push('staff'); return HttpResponse.json({ access_token: 'a1', refresh_token: 'r1', expires_in: 900, staff: {} }) }),
    http.post('/api/v1/portal/auth/refresh', () => { calls.push('portal'); return HttpResponse.json({ access_token: 'p1', refresh_token: 'pr1', expires_in: 900, user: {}, ticket_id: null }) }),
  )
  const staff = createSessionStore({ storageKey: staffKey, base: '/api/v1', refreshPath: '/auth/refresh' })
  const portal = createSessionStore({ storageKey: portalKey, base: '/api/v1/portal', refreshPath: '/auth/refresh' })
  localStorage.setItem(portalKey, 'pr0')
  expect(await portal.refreshSession()).toBe(true)
  expect(calls).toEqual(['portal'])
  expect(localStorage.getItem(portalKey)).toBe('pr1')
  expect(localStorage.getItem(staffKey)).toBeNull()
  expect(await staff.refreshSession()).toBe(false) // no staff token stored
  expect(portal.tokens.access).toBe('p1')
  expect(staff.tokens.access).toBeNull()
})

it('clears only its own key on session loss', async () => {
  server.use(http.post('/api/v1/portal/auth/refresh', () => HttpResponse.json({ error: { code: 'unauthorized', message: 'no' } }, { status: 401 })))
  const portal = createSessionStore({ storageKey: portalKey, base: '/api/v1/portal', refreshPath: '/auth/refresh' })
  const lost = vi.fn()
  portal.tokens.setOnSessionLost(lost)
  localStorage.setItem(portalKey, 'pr0')
  localStorage.setItem(staffKey, 'r0')
  expect(await portal.refreshSession()).toBe(false)
  expect(lost).toHaveBeenCalled()
  expect(localStorage.getItem(portalKey)).toBeNull()
  expect(localStorage.getItem(staffKey)).toBe('r0')
})

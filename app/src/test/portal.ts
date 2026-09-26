import { http, HttpResponse } from 'msw'
import type { PortalProfile, PortalReference, PortalSession, PortalTicket, PortalTicketRow } from '../api/types'

const profile: PortalProfile = { id: 3, email: 'pat@example.test', name: 'Pat Customer', verified: true, has_password: true }

const session: PortalSession = {
  access_token: 'paccess-1', refresh_token: 'prefresh-1', expires_in: 900, user: profile, ticket_id: null,
}

/** A ticket-scoped guest session (from an access link); refresh tokens starting `prefresh-guest` rotate into another. */
const guestSession: PortalSession = {
  access_token: 'paccess-guest-1', refresh_token: 'prefresh-guest-1', expires_in: 900,
  user: { ...profile, verified: false, has_password: false }, ticket_id: 7,
}

const reference: PortalReference = {
  site_name: 'Ticket Desk', departments: [{ id: 1, name: 'Support' }], topics: [{ id: 1, name: 'General' }],
}

const ticketRow: PortalTicketRow = {
  id: 7, number: '000007', subject: 'Printer on fire', status: { id: 1, name: 'Open' }, state: 'open',
  department: 'Support', created_at: '2026-09-25T10:00:00Z', last_message_at: '2026-09-25T10:00:00Z', closed_at: null,
}

const ticket: PortalTicket = {
  ...ticketRow, topic: 'General', updated_at: '2026-09-25T10:05:00Z',
  entries: [
    { id: 1, type: 'message', poster: 'Pat Customer', body: '<p>Help</p>', format: 'html', created_at: '2026-09-25T10:00:00Z', attachments: [] },
    { id: 2, type: 'response', poster: 'Ann Agent', body: 'On it', format: 'text', created_at: '2026-09-25T10:05:00Z',
      attachments: [{ file_id: 3, name: 'log.txt', mime: 'text/plain', size: 12 }] },
  ],
}

export const portalFixtures = { profile, session, guestSession, reference, ticketRow, ticket }

const P = '/api/v1/portal'
const unauthorized = () => HttpResponse.json({ error: { code: 'unauthorized', message: 'authentication required' } }, { status: 401 })
const notFound = () => HttpResponse.json({ error: { code: 'not_found', message: 'ticket not found' } }, { status: 404 })
// The API answers 202/201 with an empty JSON object, not an empty body.
const accepted = () => HttpResponse.json({}, { status: 202 })
const noContent = () => new HttpResponse(null, { status: 204 })

const exchanges: Record<string, PortalSession> = {
  'good-signin': { ...session, kind: 'signin' },
  'good-access': { ...guestSession, kind: 'access' },
  // Like reset, a confirm session only sets the first password: no refresh token.
  'good-confirm': { ...session, access_token: 'paccess-confirm', refresh_token: '', kind: 'confirm' },
  // A reset session may only set a password: no refresh token, so it cannot survive a reload.
  'good-reset': { ...session, access_token: 'paccess-reset', refresh_token: '', kind: 'reset' },
}

export const portalHandlers = [
  http.post(`${P}/auth/login`, async ({ request }) => {
    const body = (await request.json()) as { email: string; password: string }
    if (body.email === 'pat@example.test' && body.password === 'secret123') return HttpResponse.json(session)
    return HttpResponse.json({ error: { code: 'unauthorized', message: 'invalid email or password' } }, { status: 401 })
  }),
  http.post(`${P}/auth/link`, accepted),
  http.post(`${P}/auth/reset`, accepted),
  http.post(`${P}/access`, accepted),
  http.post(`${P}/auth/exchange`, async ({ request }) => {
    const { token } = (await request.json()) as { token: string }
    const s = exchanges[token]
    if (s) return HttpResponse.json(s)
    return HttpResponse.json({ error: { code: 'token_invalid', message: 'this link is invalid or has expired' } }, { status: 410 })
  }),
  // Always 201 {}, whether or not the address exists (no account enumeration).
  http.post(`${P}/auth/register`, () => HttpResponse.json({}, { status: 201 })),
  http.post(`${P}/auth/refresh`, async ({ request }) => {
    const { refresh_token: rt } = (await request.json()) as { refresh_token: string }
    if (rt.startsWith('prefresh-guest')) return HttpResponse.json({ ...guestSession, access_token: 'paccess-guest-2', refresh_token: 'prefresh-guest-2' })
    if (rt.startsWith('prefresh')) return HttpResponse.json({ ...session, access_token: 'paccess-2', refresh_token: 'prefresh-2' })
    return unauthorized()
  }),
  http.post(`${P}/auth/logout`, noContent),
  // Like the API: any session may read /me; a guest's answer carries the ticket it is scoped to.
  http.get(`${P}/me`, ({ request }) => (request.headers.get('Authorization')?.startsWith('Bearer paccess-guest')
    ? HttpResponse.json({ ...guestSession.user, ticket_id: guestSession.ticket_id })
    : HttpResponse.json({ ...profile, ticket_id: null }))),
  http.patch(`${P}/me`, async ({ request }) => HttpResponse.json({ ...profile, ...(await request.json() as object) })),
  http.post(`${P}/me/password`, noContent),
  http.get(`${P}/reference`, () => HttpResponse.json(reference)),
  http.post(`${P}/tickets`, () => HttpResponse.json({ id: 8, number: '000008' }, { status: 201 })),
  http.post(`${P}/files`, () => HttpResponse.json({ id: 42, name: 'a.txt', mime: 'text/plain', size: 3, token: 'tok-42' }, { status: 201 })),
  http.get(`${P}/tickets`, ({ request }) => {
    const url = new URL(request.url)
    const state = url.searchParams.get('state')
    const items = [ticketRow].filter((t) => !state || t.state === state)
    const page = Number(url.searchParams.get('page') ?? 1)
    const pageSize = Number(url.searchParams.get('page_size') ?? 25)
    return HttpResponse.json({ items: items.slice((page - 1) * pageSize, page * pageSize), page, page_size: pageSize, total: items.length })
  }),
  http.get(`${P}/tickets/:id`, ({ params }) => (params.id === '7' ? HttpResponse.json(ticket) : notFound())),
  http.post(`${P}/tickets/:id/reply`, noContent),
  http.post(`${P}/tickets/:id/close`, ({ params }) => (params.id === '7'
    ? HttpResponse.json({ ...ticket, status: { id: 3, name: 'Closed' }, state: 'closed', closed_at: '2026-09-26T10:00:00Z' })
    : notFound())),
  http.post(`${P}/tickets/:id/reopen`, ({ params }) => (params.id === '7'
    ? HttpResponse.json({ ...ticket, status: { id: 1, name: 'Open' }, state: 'open', closed_at: null })
    : notFound())),
  http.get(`${P}/tickets/:id/files/:fid`, () => new HttpResponse('hello', { headers: { 'Content-Type': 'text/plain' } })),
]

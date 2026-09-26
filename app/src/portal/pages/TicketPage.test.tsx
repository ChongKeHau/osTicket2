import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { Route, Routes } from 'react-router-dom'
import { PORTAL_REFRESH_KEY, portal } from '../../api/portalClient'
import { portalFixtures } from '../../test/portal'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/setup'
import { PortalAuthProvider } from '../PortalAuthContext'
import { PortalShell } from '../PortalShell'
import { RequirePortalUser } from '../RequirePortalUser'
import { TicketPage } from './TicketPage'

const P = '/api/v1/portal'

beforeEach(() => portal.tokens.clear())

function mount(route = '/portal/tickets/7', refresh = 'prefresh-1') {
  localStorage.setItem(PORTAL_REFRESH_KEY, refresh)
  return renderWithProviders(
    <PortalAuthProvider>
      <Routes>
        <Route path="/portal" element={<PortalShell />}>
          <Route element={<RequirePortalUser />}>
            <Route path="tickets/:id" element={<TicketPage />} />
            <Route path="tickets" element={<h2>List page</h2>} />
          </Route>
          <Route index element={<h2>Home page</h2>} />
        </Route>
      </Routes>
    </PortalAuthProvider>,
    { route },
  )
}

/** Serves ticket 7 in the given state and counts the GETs. */
function serveTicket(state: 'open' | 'closed' = 'open') {
  const calls = { n: 0 }
  const t = state === 'closed'
    ? { ...portalFixtures.ticket, state, status: { id: 3, name: 'Closed' }, closed_at: '2026-09-26T10:00:00Z' }
    : portalFixtures.ticket
  server.use(http.get(`${P}/tickets/7`, () => { calls.n++; return HttpResponse.json(t) }))
  return calls
}

it('renders the info table and the thread, messages left and responses right', async () => {
  mount()
  expect(await screen.findByRole('heading', { name: 'Printer on fire' })).toBeInTheDocument()
  const info = screen.getByRole('table', { name: 'Ticket information' })
  expect(within(info).getAllByRole('rowheader').map((th) => th.textContent))
    .toEqual(['Number', 'Status', 'Department', 'Help Topic', 'Created', 'Last Updated'])
  expect(within(info).getByText('000007')).toBeInTheDocument()
  expect(within(info).getByText('General')).toBeInTheDocument()
  const thread = screen.getByRole('region', { name: 'Thread' })
  const [msg, resp] = within(thread).getAllByRole('article')
  expect(msg).toHaveClass('message')
  expect(within(msg!).getByText('Pat Customer')).toBeInTheDocument()
  expect(within(msg!).getByText('Help')).toBeInTheDocument()
  expect(resp).toHaveClass('response')
  expect(within(resp!).getByText('Ann Agent')).toBeInTheDocument()
  expect(within(resp!).getByText('On it')).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'My Tickets' })).toHaveAttribute('href', '/portal/tickets')
})

it('downloads an attachment through the portal file route', async () => {
  Object.assign(URL, { createObjectURL: vi.fn(() => 'blob:x'), revokeObjectURL: vi.fn() })
  const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  let hit = ''
  server.use(http.get(`${P}/tickets/:id/files/:fid`, ({ request }) => {
    hit = new URL(request.url).pathname
    return new HttpResponse('hello')
  }))
  mount()
  await userEvent.click(await screen.findByRole('button', { name: /log\.txt/ }))
  await vi.waitFor(() => expect(click).toHaveBeenCalledTimes(1))
  expect(hit).toBe('/api/v1/portal/tickets/7/files/3')
  click.mockRestore()
})

it('posts a reply with attachments, refetches and flashes', async () => {
  const calls = serveTicket()
  let body: unknown
  server.use(http.post(`${P}/tickets/7/reply`, async ({ request }) => { body = await request.json(); return new HttpResponse(null, { status: 204 }) }))
  mount()
  await screen.findByRole('heading', { name: 'Printer on fire' })
  expect(screen.queryByText(/Replying will reopen this ticket/)).not.toBeInTheDocument()
  await userEvent.type(screen.getByLabelText('Reply'), 'Thanks')
  await userEvent.upload(screen.getByLabelText('Attach files'), new File(['abc'], 'a.txt', { type: 'text/plain' }))
  await screen.findByRole('button', { name: 'Remove a.txt' })
  await vi.waitFor(() => expect(screen.queryByText('uploading…')).not.toBeInTheDocument())
  const before = calls.n
  await userEvent.click(screen.getByRole('button', { name: 'Post Reply' }))
  expect(await screen.findByText('Reply posted')).toBeInTheDocument()
  expect(body).toEqual({ body: 'Thanks', format: 'text', file_ids: [42], file_tokens: ['tok-42'] })
  await vi.waitFor(() => expect(calls.n).toBeGreaterThan(before))
  expect(screen.getByLabelText('Reply')).toHaveValue('')
  expect(screen.queryByRole('button', { name: 'Remove a.txt' })).not.toBeInTheDocument()
})

it('closes the ticket only after a second, confirming click', async () => {
  serveTicket()
  let closed = 0
  server.use(http.post(`${P}/tickets/7/close`, () => { closed++; return HttpResponse.json({ ...portalFixtures.ticket, state: 'closed' }) }))
  mount()
  await userEvent.click(await screen.findByRole('button', { name: 'Close ticket' }))
  expect(closed).toBe(0)
  await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(screen.queryByRole('button', { name: 'Confirm' })).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: 'Close ticket' }))
  await userEvent.click(screen.getByRole('button', { name: 'Confirm' }))
  expect(await screen.findByText('Ticket closed')).toBeInTheDocument()
  expect(closed).toBe(1)
})

it('offers Reopen on a closed ticket and notes that replying reopens it', async () => {
  serveTicket('closed')
  let reopened = 0
  server.use(http.post(`${P}/tickets/7/reopen`, () => { reopened++; return HttpResponse.json(portalFixtures.ticket) }))
  mount()
  expect(await screen.findByText(/Replying will reopen this ticket/)).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Close ticket' })).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: 'Reopen' }))
  expect(await screen.findByText('Ticket reopened')).toBeInTheDocument()
  expect(reopened).toBe(1)
})

it('lets a guest session view and reply without a My Tickets link', async () => {
  let replied = false
  server.use(http.post(`${P}/tickets/7/reply`, () => { replied = true; return new HttpResponse(null, { status: 204 }) }))
  mount('/portal/tickets/7', 'prefresh-guest-1')
  expect(await screen.findByRole('heading', { name: 'Printer on fire' })).toBeInTheDocument()
  expect(screen.queryByRole('link', { name: 'My Tickets' })).not.toBeInTheDocument()
  await userEvent.type(screen.getByLabelText('Reply'), 'Guest reply')
  await userEvent.click(screen.getByRole('button', { name: 'Post Reply' }))
  expect(await screen.findByText('Reply posted')).toBeInTheDocument()
  expect(replied).toBe(true)
})

it('shows Ticket not found for an unknown ticket', async () => {
  mount('/portal/tickets/99')
  expect(await screen.findByRole('alert')).toHaveTextContent('Ticket not found')
})

it('a reply to a ticket that has gone flashes Ticket not found and returns to the list', async () => {
  server.use(http.post(`${P}/tickets/7/reply`, () => HttpResponse.json({ error: { code: 'not_found', message: 'ticket not found' } }, { status: 404 })))
  const { client } = mount()
  await userEvent.type(await screen.findByLabelText('Reply'), 'Hello?')
  await userEvent.click(screen.getByRole('button', { name: 'Post Reply' }))
  expect(await screen.findByRole('heading', { name: 'List page' })).toBeInTheDocument()
  expect(screen.getByRole('alert')).toHaveTextContent('Ticket not found')
  expect(client.getQueryData(['portal', 'ticket', 7])).toBeUndefined()
})

it('a close on a ticket that has gone sends a guest to the portal home', async () => {
  server.use(http.post(`${P}/tickets/7/close`, () => HttpResponse.json({ error: { code: 'not_found', message: 'ticket not found' } }, { status: 404 })))
  mount('/portal/tickets/7', 'prefresh-guest-1')
  await userEvent.click(await screen.findByRole('button', { name: 'Close ticket' }))
  await userEvent.click(screen.getByRole('button', { name: 'Confirm' }))
  expect(await screen.findByRole('heading', { name: 'Home page' })).toBeInTheDocument()
  expect(screen.getByRole('alert')).toHaveTextContent('Ticket not found')
})

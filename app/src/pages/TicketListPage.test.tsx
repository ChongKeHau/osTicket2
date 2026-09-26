import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { Route, Routes } from 'react-router-dom'
import { REFRESH_KEY } from '../api/client'
import { AuthProvider } from '../auth/AuthContext'
import { RequireAuth } from '../auth/RequireAuth'
import { server } from '../test/setup'
import { ticketFixture } from '../test/fixtures'
import { renderWithProviders } from '../test/render'
import { AppShell } from '../ui/AppShell'
import { TicketListPage } from './TicketListPage'

function mount(route: string) {
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  return renderWithProviders(
    <AuthProvider><Routes><Route element={<RequireAuth />}><Route element={<AppShell panel="agent" />}>
      <Route path="/tickets" element={<TicketListPage />} />
      <Route path="/tickets/:id" element={<h2>Detail</h2>} />
    </Route></Route></Routes></AuthProvider>,
    { route },
  )
}

it('renders the queue with osTicket columns and the sub-nav', async () => {
  mount('/tickets?state=open')
  expect(await screen.findByRole('heading', { name: /Open Tickets \(1\)/ })).toBeInTheDocument()
  const headers = screen.getAllByRole('columnheader').map((h) => h.textContent?.replace(/[▲▼]/g, '').trim())
  expect(headers).toEqual(['Number', 'Date', 'Subject', 'From', 'Priority', 'Department', 'Assigned To', 'Last Message'])
  expect(screen.getByRole('link', { name: 'Open' })).toHaveAttribute('aria-current', 'page')
  expect(screen.getByRole('link', { name: 'New Ticket' })).toHaveAttribute('href', '/tickets/new')
  const row = screen.getAllByRole('row')[1]!
  expect(within(row).getByRole('link', { name: ticketFixture.number })).toHaveAttribute('href', `/tickets/${ticketFixture.id}`)
})

it('toggles sort direction through the URL and never sends an unknown key', async () => {
  const seen: string[] = []
  server.use(http.get('/api/v1/tickets', ({ request }) => { seen.push(new URL(request.url).searchParams.get('sort') ?? ''); return HttpResponse.json({ items: [ticketFixture], page: 1, page_size: 25, total: 1 }) }))
  mount('/tickets')
  await screen.findByRole('heading', { name: /All Tickets/ })
  await userEvent.click(screen.getByRole('button', { name: /Date/ }))
  await screen.findByRole('columnheader', { name: /Date/ })
  await userEvent.click(screen.getByRole('button', { name: /Date/ }))
  expect(seen).toEqual(['-last_message_at', 'created_at', '-created_at'])
})

it('shows unanswered subjects in bold and search results title', async () => {
  server.use(http.get('/api/v1/tickets', () => HttpResponse.json({ items: [{ ...ticketFixture, is_answered: false }], page: 1, page_size: 25, total: 1 })))
  mount('/tickets?q=printer')
  expect(await screen.findByRole('heading', { name: /Search Results/ })).toBeInTheDocument()
  expect((await screen.findByText(ticketFixture.subject)).closest('td')).toHaveClass('unanswered')
})

it('renders the error banner when the list fails', async () => {
  server.use(http.get('/api/v1/tickets', () => HttpResponse.json({ error: { code: 'boom', message: 'Database down' } }, { status: 500 })))
  mount('/tickets')
  expect(await screen.findByRole('alert')).toHaveTextContent('Database down')
})

it('scopes the department filter to the agent\'s own departments', async () => {
  // The default /api/v1/me handler returns staffProfileFixture, a non-admin in department 1 only,
  // so department 2 ("Billing") must not appear in the select.
  mount('/tickets')
  await screen.findByRole('heading', { name: /All Tickets/ })
  const select = screen.getByRole('combobox', { name: 'Department' })
  await within(select).findByRole('option', { name: 'Support' })
  const options = within(select).getAllByRole('option').map((o) => o.textContent)
  expect(options).toEqual(['All departments', 'Support'])
})

it('keeps the search box in sync with the URL', async () => {
  mount('/tickets?q=printer')
  await screen.findByRole('heading', { name: /Search Results/ })
  expect(screen.getByRole('searchbox', { name: 'Search tickets' })).toHaveValue('printer')
  await userEvent.click(screen.getByRole('link', { name: 'Open' }))
  await screen.findByRole('heading', { name: /Open Tickets/ })
  expect(screen.getByRole('searchbox', { name: 'Search tickets' })).toHaveValue('')
})

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
import { RequirePortalAccount } from '../RequirePortalAccount'
import { RequirePortalUser } from '../RequirePortalUser'
import { TicketListPage } from './TicketListPage'

const P = '/api/v1/portal'

beforeEach(() => portal.tokens.clear())

function mount(route: string) {
  localStorage.setItem(PORTAL_REFRESH_KEY, 'prefresh-1')
  return renderWithProviders(
    <PortalAuthProvider>
      <Routes>
        <Route path="/portal" element={<PortalShell />}>
          <Route element={<RequirePortalUser />}>
            <Route path="tickets/:id" element={<h2>Ticket page</h2>} />
            <Route element={<RequirePortalAccount />}>
              <Route path="tickets" element={<TicketListPage />} />
            </Route>
          </Route>
        </Route>
      </Routes>
    </PortalAuthProvider>,
    { route },
  )
}

const subNav = () => within(screen.getByRole('navigation', { name: 'Secondary' }))

it('lists open tickets by default with the osTicket columns and a link to each ticket', async () => {
  const states: (string | null)[] = []
  server.use(http.get(`${P}/tickets`, ({ request }) => {
    states.push(new URL(request.url).searchParams.get('state'))
    return HttpResponse.json({ items: [portalFixtures.ticketRow], page: 1, page_size: 25, total: 1 })
  }))
  mount('/portal/tickets')
  const link = await screen.findByRole('link', { name: portalFixtures.ticketRow.number })
  expect(link).toHaveAttribute('href', '/portal/tickets/7')
  const headers = screen.getAllByRole('columnheader').map((th) => th.textContent)
  expect(headers).toEqual(['Number', 'Date', 'Subject', 'Department', 'Status', 'Last Message'])
  const table = within(screen.getByRole('table'))
  expect(table.getByText('Printer on fire')).toBeInTheDocument()
  expect(table.getByText('Support')).toBeInTheDocument()
  expect(table.getByText('Open')).toBeInTheDocument()
  expect(subNav().getByRole('link', { name: 'Open' })).toHaveAttribute('aria-current', 'page')
  expect(subNav().getByRole('link', { name: 'Closed' })).not.toHaveAttribute('aria-current')
  expect(states).toEqual(['open'])
})

it('switches to closed tickets from the sub-nav and shows the empty state', async () => {
  mount('/portal/tickets?state=open')
  await screen.findByRole('link', { name: portalFixtures.ticketRow.number })
  await userEvent.click(subNav().getByRole('link', { name: 'Closed' }))
  expect(await screen.findByText('You have no closed tickets')).toBeInTheDocument()
  expect(subNav().getByRole('link', { name: 'Closed' })).toHaveAttribute('aria-current', 'page')
  expect(subNav().getByRole('link', { name: 'Open' })).not.toHaveAttribute('aria-current')
})

it('shows the open empty state', async () => {
  server.use(http.get(`${P}/tickets`, () => HttpResponse.json({ items: [], page: 1, page_size: 25, total: 0 })))
  mount('/portal/tickets')
  expect(await screen.findByText('You have no open tickets')).toBeInTheDocument()
})

it('paginates through the footer', async () => {
  const pages: string[] = []
  server.use(http.get(`${P}/tickets`, ({ request }) => {
    const page = Number(new URL(request.url).searchParams.get('page'))
    pages.push(String(page))
    return HttpResponse.json({ items: [{ ...portalFixtures.ticketRow, id: page, number: `00000${page}` }], page, page_size: 25, total: 30 })
  }))
  mount('/portal/tickets')
  await screen.findByRole('link', { name: '000001' })
  expect(screen.getByText('Showing 1–25 of 30')).toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: 'Next' }))
  expect(await screen.findByRole('link', { name: '000002' })).toHaveAttribute('href', '/portal/tickets/2')
  expect(pages).toEqual(['1', '2'])
})

import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { PORTAL_REFRESH_KEY, portal } from '../api/portalClient'
import { renderWithProviders } from '../test/render'
import { PortalAuthProvider } from './PortalAuthContext'
import { PortalShell, usePortalSubNav } from './PortalShell'
import { RequirePortalAccount } from './RequirePortalAccount'
import { RequirePortalUser } from './RequirePortalUser'

beforeEach(() => portal.tokens.clear())

function ListPage() {
  usePortalSubNav([{ label: 'Open', to: '/portal/tickets?state=open' }, { label: 'Closed', to: '/portal/tickets?state=closed' }])
  return <h2>List page</h2>
}

function renderPortal(route: string) {
  return renderWithProviders(
    <PortalAuthProvider>
      <Routes>
        <Route path="/portal" element={<PortalShell />}>
          <Route index element={<h2>Home page</h2>} />
          <Route path="login" element={<h2>Login page</h2>} />
          <Route element={<RequirePortalUser />}>
            <Route path="tickets/:id" element={<h2>Ticket page</h2>} />
            <Route element={<RequirePortalAccount />}>
              <Route path="tickets" element={<ListPage />} />
            </Route>
          </Route>
        </Route>
      </Routes>
    </PortalAuthProvider>,
    { route },
  )
}

const tabs = () => within(screen.getByRole('navigation', { name: 'Primary' })).getAllByRole('link').map((a) => a.textContent)

it('shows the anonymous header and the three public tabs', async () => {
  renderPortal('/portal')
  expect(await screen.findByRole('heading', { name: 'Home page' })).toBeInTheDocument()
  expect(await screen.findByText('Guest User')).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Sign In' })).toHaveAttribute('href', '/portal/login')
  expect(tabs()).toEqual(['Support Center Home', 'Open a New Ticket', 'Check Ticket Status'])
  expect(screen.getByRole('link', { name: 'Support Center Home' })).toHaveAttribute('aria-current', 'page')
  expect(screen.queryByRole('button', { name: 'Sign Out' })).not.toBeInTheDocument()
})

it('shows the account header, the My Tickets tab and the page sub-nav when signed in', async () => {
  localStorage.setItem(PORTAL_REFRESH_KEY, 'prefresh-1')
  renderPortal('/portal/tickets')
  expect(await screen.findByRole('heading', { name: 'List page' })).toBeInTheDocument()
  expect(screen.getByText('Pat Customer')).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Profile' })).toHaveAttribute('href', '/portal/profile')
  expect(screen.getByRole('link', { name: 'Tickets' })).toHaveAttribute('href', '/portal/tickets')
  expect(screen.getByRole('button', { name: 'Sign Out' })).toBeInTheDocument()
  expect(tabs()).toEqual(['Support Center Home', 'Open a New Ticket', 'My Tickets'])
  expect(screen.getByRole('link', { name: 'My Tickets' })).toHaveAttribute('aria-current', 'page')
  expect(screen.getByRole('link', { name: 'Support Center Home' })).not.toHaveAttribute('aria-current')
  expect(within(screen.getByRole('navigation', { name: 'Secondary' })).getByRole('link', { name: 'Closed' })).toBeInTheDocument()
})

it('shows the guest header for a ticket-scoped session and signs out', async () => {
  localStorage.setItem(PORTAL_REFRESH_KEY, 'prefresh-guest-1')
  renderPortal('/portal/tickets/7')
  expect(await screen.findByRole('heading', { name: 'Ticket page' })).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'My Ticket' })).toHaveAttribute('href', '/portal/tickets/7')
  expect(tabs()).toEqual(['Support Center Home', 'Open a New Ticket', 'Check Ticket Status'])
  await userEvent.click(screen.getByRole('button', { name: 'Sign Out' }))
  expect(await screen.findByText('Guest User')).toBeInTheDocument()
  expect(localStorage.getItem(PORTAL_REFRESH_KEY)).toBeNull()
})

it('RequirePortalUser sends anonymous visitors to the portal sign-in page', async () => {
  renderPortal('/portal/tickets/7')
  expect(await screen.findByRole('heading', { name: 'Login page' })).toBeInTheDocument()
})

it('RequirePortalAccount sends a guest session to its ticket', async () => {
  localStorage.setItem(PORTAL_REFRESH_KEY, 'prefresh-guest-1')
  renderPortal('/portal/tickets')
  expect(await screen.findByRole('heading', { name: 'Ticket page' })).toBeInTheDocument()
  expect(screen.queryByRole('heading', { name: 'List page' })).not.toBeInTheDocument()
})

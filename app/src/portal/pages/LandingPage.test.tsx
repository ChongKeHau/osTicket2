import { screen } from '@testing-library/react'
import { PORTAL_REFRESH_KEY } from '../../api/portalClient'
import { renderWithProviders } from '../../test/render'
import { PortalAuthProvider } from '../PortalAuthContext'
import { LandingPage } from './LandingPage'

function mount() {
  return renderWithProviders(<PortalAuthProvider><LandingPage /></PortalAuthProvider>)
}

it('welcomes anonymous visitors with the two entry points', async () => {
  mount()
  expect(await screen.findByRole('heading', { name: 'Welcome to the Support Center' })).toBeInTheDocument()
  expect(screen.getByText(/open a ticket for any question/i)).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Open a New Ticket' })).toHaveAttribute('href', '/portal/open')
  expect(screen.getByRole('link', { name: 'Check Ticket Status' })).toHaveAttribute('href', '/portal/login')
})

it('offers My Tickets instead of Check Ticket Status once signed in', async () => {
  localStorage.setItem(PORTAL_REFRESH_KEY, 'prefresh-1')
  mount()
  expect(await screen.findByRole('link', { name: 'My Tickets' })).toHaveAttribute('href', '/portal/tickets')
  expect(screen.queryByRole('link', { name: 'Check Ticket Status' })).not.toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Open a New Ticket' })).toHaveAttribute('href', '/portal/open')
})

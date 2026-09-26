import { screen } from '@testing-library/react'
import App from './App'
import { signInAsAdmin } from './test/admin'
import { renderWithProviders } from './test/render'
import { server } from './test/setup'

test('unknown route renders not found', async () => {
  localStorage.setItem('ticket.refresh_token', 'refresh-1')
  renderWithProviders(<App />, { route: '/nope' })
  expect(await screen.findByText(/page not found/i)).toBeInTheDocument()
})

test('/dashboard renders inside the agent shell', async () => {
  localStorage.setItem('ticket.refresh_token', 'refresh-1')
  renderWithProviders(<App />, { route: '/dashboard' })
  expect(await screen.findByRole('heading', { name: 'Dashboard' })).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Dashboard' })).toHaveAttribute('aria-current', 'page')
})

test('/admin/email redirects to the templates list in the admin shell', async () => {
  signInAsAdmin()
  renderWithProviders(<App />, { route: '/admin/email' })
  expect(await screen.findByRole('heading', { name: /^Email Templates/ })).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Agent Panel' })).toBeInTheDocument()
})

test('/portal renders the portal without touching the staff session', async () => {
  const staffCalls: string[] = []
  server.events.on('request:start', ({ request }) => {
    const path = new URL(request.url).pathname
    if (!path.startsWith('/api/v1/portal/')) staffCalls.push(path)
  })
  localStorage.setItem('ticket.refresh_token', 'refresh-1')
  renderWithProviders(<App />, { route: '/portal' })
  expect(await screen.findByRole('heading', { name: 'Support Center' })).toBeInTheDocument()
  expect(await screen.findByText('Guest User')).toBeInTheDocument()
  server.events.removeAllListeners()
  expect(staffCalls).toEqual([])
  expect(localStorage.getItem('ticket.refresh_token')).toBe('refresh-1')
})

test('/tickets without a staff session still redirects to the staff login', async () => {
  localStorage.setItem('ticket.portal_refresh_token', 'prefresh-1')
  renderWithProviders(<App />, { route: '/tickets' })
  expect(await screen.findByText('Agent sign in')).toBeInTheDocument()
  expect(screen.queryByText('Guest User')).not.toBeInTheDocument()
})

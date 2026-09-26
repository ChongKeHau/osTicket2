import { screen } from '@testing-library/react'
import App from './App'
import { signInAsAdmin } from './test/admin'
import { renderWithProviders } from './test/render'

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

test('/admin/email redirects to the templates placeholder in the admin shell', async () => {
  signInAsAdmin()
  renderWithProviders(<App />, { route: '/admin/email' })
  expect(await screen.findByRole('heading', { name: 'Email Templates' })).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Agent Panel' })).toBeInTheDocument()
})

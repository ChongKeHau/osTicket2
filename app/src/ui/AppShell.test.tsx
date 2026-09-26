import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { REFRESH_KEY } from '../api/client'
import { AuthProvider } from '../auth/AuthContext'
import { RequireAuth } from '../auth/RequireAuth'
import { signInAsAdmin } from '../test/admin'
import { renderWithProviders } from '../test/render'
import { AppShell, useSubNav } from './AppShell'

function Page() {
  useSubNav([{ label: 'Open', to: '/tickets?state=open' }], <a href="/tickets/new">New Ticket</a>)
  return <h2>Queue</h2>
}

function app(panel: 'agent' | 'admin') {
  return (
    <AuthProvider>
      <Routes>
        <Route element={<RequireAuth />}>
          <Route element={<AppShell panel={panel} />}>
            <Route path="/tickets" element={<Page />} />
            <Route path="/admin/departments" element={<h2>Depts</h2>} />
          </Route>
        </Route>
      </Routes>
    </AuthProvider>
  )
}

it('renders the agent chrome with tabs, sub-nav and user menu', async () => {
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  renderWithProviders(app('agent'), { route: '/tickets' })
  expect(await screen.findByRole('heading', { name: 'Queue' })).toBeInTheDocument()
  expect(screen.getByRole('navigation', { name: 'Primary' })).toHaveTextContent('Dashboard')
  expect(screen.getByRole('link', { name: 'Tickets' })).toHaveAttribute('aria-current', 'page')
  expect(screen.getByRole('link', { name: 'Open' })).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'New Ticket' })).toBeInTheDocument()
  expect(screen.getByText(/Ann Agent/)).toBeInTheDocument()
  expect(screen.queryByRole('link', { name: 'Admin Panel' })).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Log Out' })).toBeInTheDocument()
})

it('shows the panel switch for admins and the admin tabs', async () => {
  signInAsAdmin()
  renderWithProviders(app('admin'), { route: '/admin/departments' })
  expect(await screen.findByRole('heading', { name: 'Depts' })).toBeInTheDocument()
  expect(screen.getByRole('navigation', { name: 'Primary' })).toHaveTextContent('Departments')
  expect(screen.getByRole('link', { name: 'Agent Panel' })).toHaveAttribute('href', '/tickets')
})

it('logs out from the header', async () => {
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  renderWithProviders(app('agent'), { route: '/tickets' })
  await screen.findByRole('heading', { name: 'Queue' })
  await userEvent.click(screen.getByRole('button', { name: 'Log Out' }))
  expect(localStorage.getItem(REFRESH_KEY)).toBeNull()
})

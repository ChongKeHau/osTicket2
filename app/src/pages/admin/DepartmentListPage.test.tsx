import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../../App'
import { REFRESH_KEY } from '../../api/client'
import { signInAsAdmin } from '../../test/admin'
import { adminFixtures } from '../../test/fixtures'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/setup'

test('a non-admin gets not found at /admin inside the agent shell with no Admin Panel link', async () => {
  localStorage.setItem(REFRESH_KEY, 'refresh-1') // default /me profile is not an admin
  renderWithProviders(<App />, { route: '/admin' })
  expect(await screen.findByRole('heading', { name: /page not found/i })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Log Out' })).toBeInTheDocument()
  expect(screen.getByRole('navigation', { name: 'Primary' })).toHaveTextContent('Tickets')
  expect(screen.queryByRole('link', { name: 'Admin Panel' })).not.toBeInTheDocument()
})

describe('as admin', () => {
  beforeEach(() => signInAsAdmin())

  test('/admin redirects to the department list with the panel switch and admin tabs', async () => {
    renderWithProviders(<App />, { route: '/admin' })
    // Anchor on the async table content (as other page tests do), not the static heading: the
    // heading renders unconditionally on mount, before the reference-data queries resolve.
    await screen.findByRole('link', { name: 'Support' })
    expect(screen.getByRole('heading', { name: 'Departments' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Agent Panel' })).toBeInTheDocument()
    const tabs = screen.getByRole('navigation', { name: 'Primary' })
    expect(within(tabs).getAllByRole('link').map((l) => l.textContent)).toEqual(['Dashboard', 'Departments', 'Help Topics', 'Staff', 'Email'])
    const rows = screen.getAllByRole('row').slice(1)
    expect(rows.map((r) => within(r).getAllByRole('cell')[0]!.textContent)).toEqual(['Support', 'Billing', 'Sales'])
    expect(within(rows[2]!).getAllByRole('cell').map((c) => c.textContent)).toEqual(['Sales', 'No', 'Ann Agent', 'Delete'])
    expect(screen.getByRole('link', { name: 'Sales' })).toHaveAttribute('href', '/admin/departments/3')
    expect(screen.getByRole('link', { name: /new department/i })).toHaveAttribute('href', '/admin/departments/new')
  })

  test('confirming a delete calls the endpoint and the row disappears', async () => {
    const deleted: string[] = []
    server.use(http.delete('/api/v1/departments/:id', ({ params }) => {
      deleted.push(String(params.id))
      server.use(http.get('/api/v1/departments', () => HttpResponse.json({ items: adminFixtures.departments.filter((d) => d.id !== 3) })))
      return new HttpResponse(null, { status: 204 })
    }))
    renderWithProviders(<App />, { route: '/admin/departments' })
    await userEvent.click(await screen.findByRole('button', { name: 'Delete Sales' }))
    await userEvent.click(screen.getByRole('button', { name: /confirm/i }))
    await waitFor(() => expect(screen.queryByRole('link', { name: 'Sales' })).not.toBeInTheDocument())
    expect(deleted).toEqual(['3'])
  })

  test('409 on delete shows the message and keeps the row', async () => {
    server.use(http.delete('/api/v1/departments/:id', () =>
      HttpResponse.json({ error: { code: 'conflict', message: 'department is referenced by tickets, staff or topics' } }, { status: 409 })))
    renderWithProviders(<App />, { route: '/admin/departments' })
    await userEvent.click(await screen.findByRole('button', { name: 'Delete Support' }))
    await userEvent.click(screen.getByRole('button', { name: /confirm/i }))
    expect(await screen.findByRole('alert')).toHaveTextContent('department is referenced by tickets, staff or topics')
    expect(screen.getByRole('link', { name: 'Support' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Delete Support' })).toBeInTheDocument()
  })

  test('a failed department list shows a retryable banner', async () => {
    server.use(http.get('/api/v1/departments', () => HttpResponse.json({ error: { code: 'server_error', message: 'boom' } }, { status: 500 })))
    renderWithProviders(<App />, { route: '/admin/departments' })
    const retry = await screen.findByRole('button', { name: /retry/i })
    server.use(http.get('/api/v1/departments', () => HttpResponse.json({ items: adminFixtures.departments })))
    await userEvent.click(retry)
    expect(await screen.findByRole('link', { name: 'Sales' })).toBeInTheDocument()
  })
})

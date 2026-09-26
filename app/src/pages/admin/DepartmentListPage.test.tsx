import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../../App'
import { REFRESH_KEY } from '../../api/client'
import { signInAsAdmin } from '../../test/admin'
import { adminFixtures } from '../../test/fixtures'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/setup'

const rowOf = (name: string) => screen.getByRole('link', { name }).closest('tr')!
const bodyRows = () => within(screen.getAllByRole('rowgroup')[1]!).getAllByRole('row')
const firstCells = () => bodyRows().map((r) => within(r).getAllByRole('cell')[0]!.textContent)

async function askDelete(name: string) {
  await userEvent.click(within(rowOf(name)).getByRole('button', { name: 'More' }))
  await userEvent.click(screen.getByRole('menuitem', { name: 'Delete' }))
}

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

  test('/admin redirects to the department list with the panel switch, admin tabs, sub-nav and sticky bar', async () => {
    renderWithProviders(<App />, { route: '/admin' })
    await screen.findByRole('link', { name: 'Support' })
    expect(screen.getByRole('heading', { name: /^Departments/ })).toHaveTextContent('Departments (3)')
    expect(screen.getByRole('link', { name: 'Agent Panel' })).toBeInTheDocument()
    const tabs = screen.getByRole('navigation', { name: 'Primary' })
    expect(within(tabs).getAllByRole('link').map((l) => l.textContent)).toEqual(['Dashboard', 'Departments', 'Help Topics', 'Staff', 'Email'])
    const sub = screen.getByRole('navigation', { name: 'Secondary' })
    expect(within(sub).getByRole('link', { name: 'All Departments' })).toHaveAttribute('href', '/admin/departments')
    expect(within(sub).queryByRole('link', { name: /Add New/ })).not.toBeInTheDocument()
    const add = screen.getAllByRole('link', { name: /Add New/ })
    expect(add).toHaveLength(1)
    expect(add[0]).toHaveTextContent('Add New Department')
    expect(add[0]).toHaveAttribute('href', '/admin/departments/new')
    // Sorted by name by default.
    expect(firstCells()).toEqual(['Billing', 'Sales', 'Support'])
    expect(within(rowOf('Sales')).getAllByRole('cell').map((c) => c.textContent)).toEqual(['Sales', 'Private', 'Ann Agent', 'More ▾'])
    expect(screen.getByRole('link', { name: 'Sales' })).toHaveAttribute('href', '/admin/departments/3')
    expect(screen.getByText('3 departments')).toBeInTheDocument()
  })

  test('sorts client-side by name', async () => {
    renderWithProviders(<App />, { route: '/admin/departments' })
    await screen.findByRole('link', { name: 'Support' })
    const before = firstCells()
    await userEvent.click(screen.getByRole('button', { name: /Name/ }))
    expect(firstCells()).toEqual([...before].reverse())
    await userEvent.click(screen.getByRole('button', { name: /Name/ }))
    expect(firstCells()).toEqual(before)
    await userEvent.click(screen.getByRole('button', { name: /Type/ }))
    expect(firstCells()[0]).toBe('Sales') // private (false) sorts first
  })

  test('confirming a delete from the More menu calls the endpoint, flashes, and the row disappears', async () => {
    const deleted: string[] = []
    server.use(http.delete('/api/v1/departments/:id', ({ params }) => {
      deleted.push(String(params.id))
      server.use(http.get('/api/v1/departments', () => HttpResponse.json({ items: adminFixtures.departments.filter((d) => d.id !== 3) })))
      return new HttpResponse(null, { status: 204 })
    }))
    renderWithProviders(<App />, { route: '/admin/departments' })
    await screen.findByRole('link', { name: 'Sales' })
    await askDelete('Sales')
    await userEvent.click(screen.getByRole('button', { name: 'Confirm delete Sales' }))
    await waitFor(() => expect(screen.queryByRole('link', { name: 'Sales' })).not.toBeInTheDocument())
    expect(deleted).toEqual(['3'])
    expect(screen.getByRole('status')).toHaveTextContent('Department "Sales" deleted')
  })

  test('cancelling the confirm puts the More menu back', async () => {
    renderWithProviders(<App />, { route: '/admin/departments' })
    await screen.findByRole('link', { name: 'Sales' })
    await askDelete('Sales')
    await userEvent.click(screen.getByRole('button', { name: 'Cancel delete Sales' }))
    expect(within(rowOf('Sales')).getByRole('button', { name: 'More' })).toBeInTheDocument()
  })

  test('409 on delete shows the message and keeps the row', async () => {
    server.use(http.delete('/api/v1/departments/:id', () =>
      HttpResponse.json({ error: { code: 'conflict', message: 'department is referenced by tickets, staff or topics' } }, { status: 409 })))
    renderWithProviders(<App />, { route: '/admin/departments' })
    await screen.findByRole('link', { name: 'Support' })
    await askDelete('Support')
    await userEvent.click(screen.getByRole('button', { name: 'Confirm delete Support' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('department is referenced by tickets, staff or topics')
    expect(screen.getByRole('link', { name: 'Support' })).toBeInTheDocument()
    expect(within(rowOf('Support')).getByRole('button', { name: 'More' })).toBeInTheDocument()
  })

  test('a failed department list shows a retryable banner', async () => {
    server.use(http.get('/api/v1/departments', () => HttpResponse.json({ error: { code: 'server_error', message: 'boom' } }, { status: 500 })))
    renderWithProviders(<App />, { route: '/admin/departments' })
    const retry = await screen.findByRole('button', { name: /retry/i })
    expect(screen.getByRole('alert')).toHaveTextContent('boom')
    server.use(http.get('/api/v1/departments', () => HttpResponse.json({ items: adminFixtures.departments })))
    await userEvent.click(retry)
    expect(await screen.findByRole('link', { name: 'Sales' })).toBeInTheDocument()
  })
})

import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../../App'
import { signInAsAdmin } from '../../test/admin'
import { adminFixtures } from '../../test/fixtures'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/setup'

beforeEach(() => signInAsAdmin())

const bodyRows = () => within(screen.getAllByRole('rowgroup')[1]!).getAllByRole('row')
const cells = (r: HTMLElement) => within(r).getAllByRole('cell').map((c) => c.textContent)

test('list shows staff with role and status, sorted by username, and no delete', async () => {
  renderWithProviders(<App />, { route: '/admin/staff' })
  expect(await screen.findByRole('link', { name: /Olive Old/ })).toBeInTheDocument()
  expect(screen.getByRole('heading', { name: /^Staff/ })).toHaveTextContent('Staff (4)')
  const sub = screen.getByRole('navigation', { name: 'Secondary' })
  expect(within(sub).getByRole('link', { name: 'All Staff' })).toHaveAttribute('href', '/admin/staff')
  expect(within(sub).getByRole('link', { name: 'Add New Staff Member' })).toHaveAttribute('href', '/admin/staff/new')
  const rows = bodyRows()
  expect(rows.map((r) => cells(r)[1])).toEqual(['agent', 'bob', 'old', 'root'])
  expect(cells(rows[3]!)).toEqual(['Root Admin', 'root', 'root@example.test', 'Administrator', 'Active', 'Support'])
  expect(cells(rows[2]!)).toEqual(['Olive Old', 'old', 'old@example.test', 'Agent', 'Inactive', 'Billing'])
  expect(screen.queryByRole('button', { name: 'More' })).not.toBeInTheDocument()
  expect(screen.getByRole('link', { name: /Olive Old/ })).toHaveAttribute('href', '/admin/staff/4')
})

test('list sorts by role', async () => {
  renderWithProviders(<App />, { route: '/admin/staff' })
  await screen.findByRole('link', { name: /Olive Old/ })
  await userEvent.click(screen.getByRole('button', { name: /Role/ }))
  await userEvent.click(screen.getByRole('button', { name: /Role/ }))
  expect(cells(bodyRows()[0]!)[0]).toBe('Root Admin')
})

test('create shows username and password, adds the primary department to memberships', async () => {
  let body: unknown
  server.use(http.post('/api/v1/staff', async ({ request }) => { body = await request.json(); return HttpResponse.json({ id: 9, is_active: true, ...(body as object) }, { status: 201 }) }))
  renderWithProviders(<App />, { route: '/admin/staff/new' })
  expect(await screen.findByRole('heading', { name: 'Add New Staff Member' })).toBeInTheDocument()
  for (const section of ['Account', 'Permissions', 'Departments']) expect(screen.getByText(section, { selector: 'th' })).toBeInTheDocument()
  await userEvent.type(screen.getByLabelText(/^Username/), 'newbie')
  await userEvent.type(screen.getByLabelText(/^Email/), 'newbie@example.test')
  await userEvent.type(screen.getByLabelText(/^Password/), 'secret123')
  await userEvent.type(screen.getByLabelText(/^First name/), 'New')
  await userEvent.type(screen.getByLabelText(/^Last name/), 'Person')
  await userEvent.click(screen.getByLabelText(/^Administrator/))
  await userEvent.selectOptions(screen.getByLabelText(/^Primary department/), '2')
  await userEvent.selectOptions(screen.getByLabelText(/^Additional departments/), '3')
  expect(screen.queryByLabelText(/^Active/)).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Change Password' })).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(body).toEqual({
    username: 'newbie', email: 'newbie@example.test', password: 'secret123', first_name: 'New', last_name: 'Person',
    is_admin: true, primary_dept_id: 2, department_ids: [3, 2],
  }))
  expect(await screen.findByRole('heading', { name: /^Staff/ })).toBeInTheDocument()
  expect(screen.getByRole('status')).toHaveTextContent('Staff member saved')
})

test('edit shows username read-only, seeds fields, sends only changes, and stays', async () => {
  let body: unknown
  server.use(http.patch('/api/v1/staff/2', async ({ request }) => { body = await request.json(); return HttpResponse.json({ ...adminFixtures.staff[1], ...(body as object) }) }))
  renderWithProviders(<App />, { route: '/admin/staff/2' })
  expect(await screen.findByRole('heading', { name: 'Staff Member: Bob Billing' })).toBeInTheDocument()
  expect(screen.queryByLabelText(/^Username/)).not.toBeInTheDocument()
  expect(screen.getByText('bob')).toBeInTheDocument()
  expect(screen.getByLabelText(/^Email/)).toHaveValue('bob@example.test')
  expect(screen.getByLabelText(/^Primary department/)).toHaveValue('2')
  expect(screen.getByLabelText(/^Additional departments/)).toHaveValue(['2'])
  expect(screen.getByLabelText(/^Active/)).toBeChecked()
  await userEvent.click(screen.getByLabelText(/^Active/))
  await userEvent.selectOptions(screen.getByLabelText(/^Additional departments/), '1')
  await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() => expect(body).toEqual({ is_active: false, department_ids: [1, 2] }))
  expect(await screen.findByRole('status')).toHaveTextContent('Staff member saved')
  expect(screen.getByRole('heading', { name: 'Staff Member: Bob Billing' })).toBeInTheDocument()
})

test('reset restores the loaded values', async () => {
  renderWithProviders(<App />, { route: '/admin/staff/2' })
  const email = await screen.findByLabelText(/^Email/)
  await userEvent.clear(email)
  await userEvent.type(email, 'x@example.test')
  await userEvent.click(screen.getByLabelText(/^Administrator/))
  await userEvent.click(screen.getByRole('button', { name: 'Reset' }))
  expect(email).toHaveValue('bob@example.test')
  expect(screen.getByLabelText(/^Administrator/)).not.toBeChecked()
})

test('primary department is always included in memberships', async () => {
  let body: unknown
  server.use(http.patch('/api/v1/staff/2', async ({ request }) => { body = await request.json(); return HttpResponse.json(adminFixtures.staff[1]) }))
  renderWithProviders(<App />, { route: '/admin/staff/2' })
  await userEvent.deselectOptions(await screen.findByLabelText(/^Additional departments/), '2')
  await userEvent.selectOptions(screen.getByLabelText(/^Primary department/), '1')
  await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() => expect(body).toEqual({ primary_dept_id: 1, department_ids: [1] }))
})

test('change password posts to the password endpoint and confirms', async () => {
  let body: unknown
  server.use(http.post('/api/v1/staff/2/password', async ({ request }) => { body = await request.json(); return new HttpResponse(null, { status: 204 }) }))
  renderWithProviders(<App />, { route: '/admin/staff/2' })
  await userEvent.type(await screen.findByLabelText(/^Password/), 'another123')
  await userEvent.click(screen.getByRole('button', { name: 'Change Password' }))
  expect(await screen.findByRole('status')).toHaveTextContent('Password updated')
  expect(body).toEqual({ password: 'another123' })
  expect(screen.getByLabelText(/^Password/)).toHaveValue('')
})

test('a change-password validation error renders in its row', async () => {
  server.use(http.post('/api/v1/staff/2/password', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { password: 'too short' } } }, { status: 400 })))
  renderWithProviders(<App />, { route: '/admin/staff/2' })
  await userEvent.type(await screen.findByLabelText(/^Password/), 'x')
  await userEvent.click(screen.getByRole('button', { name: 'Change Password' }))
  expect(await screen.findByText('too short')).toBeInTheDocument()
  expect(screen.getByLabelText(/^Password/)).toHaveAttribute('aria-invalid', 'true')
})

test('the last-admin conflict shows in the banner', async () => {
  server.use(http.patch('/api/v1/staff/3', () => HttpResponse.json({ error: { code: 'conflict', message: 'cannot remove the last active admin' } }, { status: 409 })))
  renderWithProviders(<App />, { route: '/admin/staff/3' })
  await userEvent.click(await screen.findByLabelText(/^Administrator/))
  await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('cannot remove the last active admin')
})

test('create validation errors render in their rows', async () => {
  server.use(http.post('/api/v1/staff', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { email: 'email', primary_dept_id: 'required' } } }, { status: 400 })))
  renderWithProviders(<App />, { route: '/admin/staff/new' })
  await userEvent.click(await screen.findByRole('button', { name: 'Create' }))
  expect(await screen.findByText('email')).toBeInTheDocument()
  expect(screen.getByText('required')).toBeInTheDocument()
  expect(screen.getByLabelText(/^Primary department/)).toHaveAttribute('aria-invalid', 'true')
})

test('an is_active error on create goes to the banner, since create does not render that field', async () => {
  server.use(http.post('/api/v1/staff', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { is_active: 'bad' } } }, { status: 400 })))
  renderWithProviders(<App />, { route: '/admin/staff/new' })
  await userEvent.click(await screen.findByRole('button', { name: 'Create' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('request validation failed')
})

test('unknown id renders not found', async () => {
  renderWithProviders(<App />, { route: '/admin/staff/999' })
  expect(await screen.findByRole('heading', { name: /page not found/i })).toBeInTheDocument()
})

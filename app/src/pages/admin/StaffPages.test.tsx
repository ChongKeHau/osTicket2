import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../../App'
import { signInAsAdmin } from '../../test/admin'
import { adminFixtures } from '../../test/fixtures'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/setup'

beforeEach(() => signInAsAdmin())

test('list shows staff with an Inactive badge and no delete buttons', async () => {
  renderWithProviders(<App />, { route: '/admin/staff' })
  expect(await screen.findByRole('link', { name: /Olive Old/ })).toBeInTheDocument()
  expect(screen.getByRole('heading', { name: 'Staff' })).toBeInTheDocument()
  const rows = screen.getAllByRole('row').slice(1)
  expect(rows).toHaveLength(4)
  expect(within(rows[2]!).getAllByRole('cell').map((c) => c.textContent)).toEqual(['Root Admin', 'root', 'root@example.test', 'Yes', 'Yes', 'Support'])
  expect(within(rows[3]!).getByText('Inactive')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /^delete/i })).not.toBeInTheDocument()
  expect(screen.getByRole('link', { name: /Olive Old/ })).toHaveAttribute('href', '/admin/staff/4')
})

test('create shows username and password, adds the primary department to memberships', async () => {
  let body: unknown
  server.use(http.post('/api/v1/staff', async ({ request }) => { body = await request.json(); return HttpResponse.json({ id: 9, is_active: true, ...(body as object) }, { status: 201 }) }))
  renderWithProviders(<App />, { route: '/admin/staff/new' })
  await userEvent.type(await screen.findByLabelText(/^username$/i), 'newbie')
  await userEvent.type(screen.getByLabelText(/^email$/i), 'newbie@example.test')
  await userEvent.type(screen.getByLabelText(/^password$/i), 'secret123')
  await userEvent.type(screen.getByLabelText(/first name/i), 'New')
  await userEvent.type(screen.getByLabelText(/last name/i), 'Person')
  await userEvent.click(screen.getByLabelText(/^administrator$/i))
  await userEvent.selectOptions(screen.getByLabelText(/primary department/i), '2')
  await userEvent.click(screen.getByLabelText('Sales')) // membership checkbox; Billing (primary) is not ticked
  expect(screen.queryByLabelText(/^active$/i)).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: /^create$/i }))
  await waitFor(() => expect(body).toEqual({
    username: 'newbie', email: 'newbie@example.test', password: 'secret123', first_name: 'New', last_name: 'Person',
    is_admin: true, primary_dept_id: 2, department_ids: [3, 2],
  }))
  expect(await screen.findByRole('heading', { name: 'Staff' })).toBeInTheDocument()
})

test('edit hides username and password, seeds fields, and sends only changes', async () => {
  let body: unknown
  server.use(http.patch('/api/v1/staff/2', async ({ request }) => { body = await request.json(); return HttpResponse.json({ ...adminFixtures.staff[1], ...(body as object) }) }))
  renderWithProviders(<App />, { route: '/admin/staff/2' })
  expect(await screen.findByRole('heading', { name: 'Edit Bob Billing' })).toBeInTheDocument()
  expect(screen.queryByLabelText(/^username$/i)).not.toBeInTheDocument()
  expect(screen.queryByLabelText(/^password$/i)).not.toBeInTheDocument()
  expect(screen.getByLabelText(/^email$/i)).toHaveValue('bob@example.test')
  expect(screen.getByLabelText(/primary department/i)).toHaveValue('2')
  expect(screen.getByLabelText('Billing')).toBeChecked()
  expect(screen.getByLabelText(/^active$/i)).toBeChecked()
  await userEvent.click(screen.getByLabelText(/^active$/i))
  await userEvent.click(screen.getByLabelText('Support'))
  await userEvent.click(screen.getByRole('button', { name: /^save$/i }))
  await waitFor(() => expect(body).toEqual({ is_active: false, department_ids: [2, 1] }))
})

test('primary department is always included in memberships', async () => {
  let body: unknown
  server.use(http.patch('/api/v1/staff/2', async ({ request }) => { body = await request.json(); return HttpResponse.json(adminFixtures.staff[1]) }))
  renderWithProviders(<App />, { route: '/admin/staff/2' })
  await userEvent.click(await screen.findByLabelText('Billing')) // untick the primary department
  await userEvent.selectOptions(screen.getByLabelText(/primary department/i), '1')
  await userEvent.click(screen.getByRole('button', { name: /^save$/i }))
  await waitFor(() => expect(body).toEqual({ primary_dept_id: 1, department_ids: [1] }))
})

test('set password posts to the password endpoint and confirms', async () => {
  let body: unknown
  server.use(http.post('/api/v1/staff/2/password', async ({ request }) => { body = await request.json(); return new HttpResponse(null, { status: 204 }) }))
  renderWithProviders(<App />, { route: '/admin/staff/2' })
  await userEvent.type(await screen.findByLabelText(/new password/i), 'another123')
  await userEvent.click(screen.getByRole('button', { name: /set password/i }))
  expect(await screen.findByText('Password updated')).toBeInTheDocument()
  expect(body).toEqual({ password: 'another123' })
  expect(screen.getByLabelText(/new password/i)).toHaveValue('')
})

test('the last-admin conflict shows in the banner', async () => {
  server.use(http.patch('/api/v1/staff/3', () => HttpResponse.json({ error: { code: 'conflict', message: 'cannot remove the last active admin' } }, { status: 409 })))
  renderWithProviders(<App />, { route: '/admin/staff/3' })
  await userEvent.click(await screen.findByLabelText(/^administrator$/i))
  await userEvent.click(screen.getByRole('button', { name: /^save$/i }))
  expect(await screen.findByRole('alert')).toHaveTextContent('cannot remove the last active admin')
})

test('create validation errors render under fields', async () => {
  server.use(http.post('/api/v1/staff', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { email: 'email', primary_dept_id: 'required' } } }, { status: 400 })))
  renderWithProviders(<App />, { route: '/admin/staff/new' })
  await userEvent.click(await screen.findByRole('button', { name: /^create$/i }))
  expect(await screen.findByText('email')).toBeInTheDocument()
  expect(screen.getByText('required')).toBeInTheDocument()
})

test('unknown id renders not found', async () => {
  renderWithProviders(<App />, { route: '/admin/staff/999' })
  expect(await screen.findByRole('heading', { name: /page not found/i })).toBeInTheDocument()
})

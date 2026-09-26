import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../../App'
import { signInAsAdmin } from '../../test/admin'
import { adminFixtures } from '../../test/fixtures'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/setup'

beforeEach(() => signInAsAdmin())

const bannerAlerts = () => screen.queryAllByRole('alert').filter((a) => a.textContent?.includes('request validation failed'))

test('create submits the form, flashes, and returns to the list', async () => {
  let body: unknown
  server.use(http.post('/api/v1/departments', async ({ request }) => { body = await request.json(); return HttpResponse.json({ id: 9, ...(body as object) }, { status: 201 }) }))
  renderWithProviders(<App />, { route: '/admin/departments/new' })
  expect(await screen.findByRole('heading', { name: 'Add New Department' })).toBeInTheDocument()
  expect(screen.getByText('Settings')).toBeInTheDocument()
  expect(screen.getByText('Public departments are visible to end users')).toBeInTheDocument()
  await userEvent.type(screen.getByLabelText(/^Name/), 'Sales EMEA')
  await userEvent.click(screen.getByLabelText(/^Public/))
  await userEvent.selectOptions(screen.getByLabelText(/^Manager/), '2')
  await userEvent.click(screen.getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(body).toEqual({ name: 'Sales EMEA', is_public: false, manager_id: 2 }))
  expect(await screen.findByRole('heading', { name: /^Departments/ })).toBeInTheDocument()
  expect(screen.getByRole('status')).toHaveTextContent('Department saved')
})

test('manager offers active staff only', async () => {
  renderWithProviders(<App />, { route: '/admin/departments/new' })
  const manager = await screen.findByLabelText(/^Manager/)
  const options = Array.from((manager as HTMLSelectElement).options).map((o) => o.textContent)
  expect(options).toEqual(['— none —', 'Ann Agent', 'Bob Billing', 'Root Admin'])
})

test('edit seeds the form, sends only changed fields, and stays on the form', async () => {
  let body: unknown
  server.use(http.patch('/api/v1/departments/3', async ({ request }) => { body = await request.json(); return HttpResponse.json({ ...adminFixtures.departments[2], ...(body as object) }) }))
  renderWithProviders(<App />, { route: '/admin/departments/3' })
  expect(await screen.findByRole('heading', { name: 'Department: Sales' })).toBeInTheDocument()
  expect(screen.getByLabelText(/^Name/)).toHaveValue('Sales')
  expect(screen.getByLabelText(/^Public/)).not.toBeChecked()
  expect(screen.getByLabelText(/^Manager/)).toHaveValue('1')
  await userEvent.clear(screen.getByLabelText(/^Name/))
  await userEvent.type(screen.getByLabelText(/^Name/), 'Sales EMEA')
  await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() => expect(body).toEqual({ name: 'Sales EMEA' }))
  expect(await screen.findByRole('status')).toHaveTextContent('Department saved')
  expect(screen.getByLabelText(/^Name/)).toHaveValue('Sales EMEA')
})

test('resets edits to the loaded record', async () => {
  renderWithProviders(<App />, { route: '/admin/departments/1' })
  const name = await screen.findByLabelText(/^Name/)
  await userEvent.clear(name)
  await userEvent.type(name, 'Changed')
  await userEvent.click(screen.getByLabelText(/^Public/))
  await userEvent.click(screen.getByRole('button', { name: 'Reset' }))
  expect(name).toHaveValue(adminFixtures.departments[0]!.name)
  expect(screen.getByLabelText(/^Public/)).toBeChecked()
})

test('no changes sends no PATCH and says so', async () => {
  let calls = 0
  server.use(http.patch('/api/v1/departments/:id', () => { calls++; return HttpResponse.json(adminFixtures.departments[0]) }))
  renderWithProviders(<App />, { route: '/admin/departments/1' })
  await userEvent.click(await screen.findByRole('button', { name: 'Save Changes' }))
  expect(await screen.findByRole('status')).toHaveTextContent('No changes to save')
  expect(calls).toBe(0)
})

test('cancel returns to the list', async () => {
  renderWithProviders(<App />, { route: '/admin/departments/1' })
  await userEvent.click(await screen.findByRole('link', { name: 'Cancel' }))
  expect(await screen.findByRole('heading', { name: /^Departments/ })).toBeInTheDocument()
})

test('clearing the manager sends manager_id null', async () => {
  let body: unknown
  server.use(http.patch('/api/v1/departments/3', async ({ request }) => { body = await request.json(); return HttpResponse.json(adminFixtures.departments[2]) }))
  renderWithProviders(<App />, { route: '/admin/departments/3' })
  await userEvent.selectOptions(await screen.findByLabelText(/^Manager/), '')
  await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() => expect(body).toEqual({ manager_id: null }))
})

test('a known field error renders in its row; an unknown one falls back to the banner', async () => {
  server.use(http.post('/api/v1/departments', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { name: 'required' } } }, { status: 400 })))
  renderWithProviders(<App />, { route: '/admin/departments/new' })
  await userEvent.click(await screen.findByRole('button', { name: 'Create' }))
  expect(await screen.findByText('required')).toBeInTheDocument()
  expect(screen.getByLabelText(/^Name/)).toHaveAttribute('aria-invalid', 'true')
  expect(bannerAlerts()).toHaveLength(0)
  server.use(http.post('/api/v1/departments', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { something: 'bad' } } }, { status: 400 })))
  await userEvent.click(screen.getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(bannerAlerts()).toHaveLength(1))
})

test('a conflict shows in the banner and keeps the form', async () => {
  server.use(http.post('/api/v1/departments', () =>
    HttpResponse.json({ error: { code: 'conflict', message: 'department name already exists' } }, { status: 409 })))
  renderWithProviders(<App />, { route: '/admin/departments/new' })
  await userEvent.type(await screen.findByLabelText(/^Name/), 'Support')
  await userEvent.click(screen.getByRole('button', { name: 'Create' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('department name already exists')
  expect(screen.getByLabelText(/^Name/)).toHaveValue('Support')
})

test('a 404 on save shows the banner and keeps the edits although the record left the list', async () => {
  server.use(http.patch('/api/v1/departments/3', () => {
    server.use(http.get('/api/v1/departments', () => HttpResponse.json({ items: adminFixtures.departments.filter((d) => d.id !== 3) })))
    return HttpResponse.json({ error: { code: 'not_found', message: 'department not found' } }, { status: 404 })
  }))
  renderWithProviders(<App />, { route: '/admin/departments/3' })
  await userEvent.clear(await screen.findByLabelText(/^Name/))
  await userEvent.type(screen.getByLabelText(/^Name/), 'Sales EMEA')
  await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('department not found')
  // Let the post-mutation refetch land, then check the form survived it.
  await waitFor(() => expect(screen.queryByRole('heading', { name: /page not found/i })).not.toBeInTheDocument())
  expect(screen.getByRole('alert')).toHaveTextContent('department not found')
  expect(screen.getByLabelText(/^Name/)).toHaveValue('Sales EMEA')
})

test('a reference-data failure offers Retry, which loads the form', async () => {
  server.use(http.get('/api/v1/departments', () => HttpResponse.json({ error: { code: 'internal', message: 'internal server error' } }, { status: 500 })))
  renderWithProviders(<App />, { route: '/admin/departments/new' })
  const retry = await screen.findByRole('button', { name: /retry/i })
  server.use(http.get('/api/v1/departments', () => HttpResponse.json({ items: adminFixtures.departments })))
  await userEvent.click(retry)
  expect(await screen.findByRole('heading', { name: 'Add New Department' })).toBeInTheDocument()
})

test('unknown or invalid id renders not found', async () => {
  renderWithProviders(<App />, { route: '/admin/departments/999' })
  expect(await screen.findByRole('heading', { name: /page not found/i })).toBeInTheDocument()
})

test('a non-numeric id renders not found', async () => {
  renderWithProviders(<App />, { route: '/admin/departments/abc' })
  expect(await screen.findByRole('heading', { name: /page not found/i })).toBeInTheDocument()
})

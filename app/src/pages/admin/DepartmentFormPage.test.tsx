import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../../App'
import { signInAsAdmin } from '../../test/admin'
import { adminFixtures } from '../../test/fixtures'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/setup'

beforeEach(() => signInAsAdmin())

test('create submits the form and returns to the list', async () => {
  let body: unknown
  server.use(http.post('/api/v1/departments', async ({ request }) => { body = await request.json(); return HttpResponse.json({ id: 9, ...(body as object) }, { status: 201 }) }))
  renderWithProviders(<App />, { route: '/admin/departments/new' })
  await userEvent.type(await screen.findByLabelText(/^name$/i), 'Sales EMEA')
  await userEvent.click(screen.getByLabelText(/^public$/i))
  await userEvent.selectOptions(screen.getByLabelText(/^manager$/i), '2')
  await userEvent.click(screen.getByRole('button', { name: /^create$/i }))
  await waitFor(() => expect(body).toEqual({ name: 'Sales EMEA', is_public: false, manager_id: 2 }))
  expect(await screen.findByRole('heading', { name: 'Departments' })).toBeInTheDocument()
})

test('edit seeds the form and sends only changed fields', async () => {
  let body: unknown
  server.use(http.patch('/api/v1/departments/3', async ({ request }) => { body = await request.json(); return HttpResponse.json({ ...adminFixtures.departments[2], ...(body as object) }) }))
  renderWithProviders(<App />, { route: '/admin/departments/3' })
  expect(await screen.findByRole('heading', { name: 'Edit Sales' })).toBeInTheDocument()
  expect(screen.getByLabelText(/^name$/i)).toHaveValue('Sales')
  expect(screen.getByLabelText(/^public$/i)).not.toBeChecked()
  expect(screen.getByLabelText(/^manager$/i)).toHaveValue('1')
  await userEvent.clear(screen.getByLabelText(/^name$/i))
  await userEvent.type(screen.getByLabelText(/^name$/i), 'Sales EMEA')
  await userEvent.click(screen.getByRole('button', { name: /^save$/i }))
  await waitFor(() => expect(body).toEqual({ name: 'Sales EMEA' }))
  expect(await screen.findByRole('heading', { name: 'Departments' })).toBeInTheDocument()
})

test('no changes navigates back without a PATCH', async () => {
  let calls = 0
  server.use(http.patch('/api/v1/departments/:id', () => { calls++; return HttpResponse.json(adminFixtures.departments[0]) }))
  renderWithProviders(<App />, { route: '/admin/departments/1' })
  await userEvent.click(await screen.findByRole('button', { name: /^save$/i }))
  expect(await screen.findByRole('heading', { name: 'Departments' })).toBeInTheDocument()
  expect(calls).toBe(0)
})

test('clearing the manager sends manager_id null', async () => {
  let body: unknown
  server.use(http.patch('/api/v1/departments/3', async ({ request }) => { body = await request.json(); return HttpResponse.json(adminFixtures.departments[2]) }))
  renderWithProviders(<App />, { route: '/admin/departments/3' })
  await userEvent.selectOptions(await screen.findByLabelText(/^manager$/i), '')
  await userEvent.click(screen.getByRole('button', { name: /^save$/i }))
  await waitFor(() => expect(body).toEqual({ manager_id: null }))
})

test('a known field error renders under the field; an unknown one falls back to the banner', async () => {
  server.use(http.post('/api/v1/departments', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { name: 'required' } } }, { status: 400 })))
  renderWithProviders(<App />, { route: '/admin/departments/new' })
  await userEvent.click(await screen.findByRole('button', { name: /^create$/i }))
  expect(await screen.findByText('required')).toBeInTheDocument()
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  server.use(http.post('/api/v1/departments', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { something: 'bad' } } }, { status: 400 })))
  await userEvent.click(screen.getByRole('button', { name: /^create$/i }))
  expect(await screen.findByRole('alert')).toHaveTextContent('request validation failed')
})

test('a conflict shows in the banner and keeps the form', async () => {
  server.use(http.post('/api/v1/departments', () =>
    HttpResponse.json({ error: { code: 'conflict', message: 'department name already exists' } }, { status: 409 })))
  renderWithProviders(<App />, { route: '/admin/departments/new' })
  await userEvent.type(await screen.findByLabelText(/^name$/i), 'Support')
  await userEvent.click(screen.getByRole('button', { name: /^create$/i }))
  expect(await screen.findByRole('alert')).toHaveTextContent('department name already exists')
  expect(screen.getByLabelText(/^name$/i)).toHaveValue('Support')
})

test('unknown or invalid id renders not found', async () => {
  renderWithProviders(<App />, { route: '/admin/departments/999' })
  expect(await screen.findByRole('heading', { name: /page not found/i })).toBeInTheDocument()
})

test('a non-numeric id renders not found', async () => {
  renderWithProviders(<App />, { route: '/admin/departments/abc' })
  expect(await screen.findByRole('heading', { name: /page not found/i })).toBeInTheDocument()
})

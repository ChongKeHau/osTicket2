import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../../App'
import { signInAsAdmin } from '../../test/admin'
import { referenceFixtures } from '../../test/fixtures'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/setup'

beforeEach(() => signInAsAdmin())

test('list shows topics with department, priority, active flag, and sort order', async () => {
  renderWithProviders(<App />, { route: '/admin/topics' })
  expect(await screen.findByRole('link', { name: 'Refunds' })).toBeInTheDocument()
  expect(screen.getByRole('heading', { name: 'Topics' })).toBeInTheDocument()
  const rows = screen.getAllByRole('row').slice(1)
  expect(rows).toHaveLength(2)
  expect(within(rows[1]!).getAllByRole('cell').map((c) => c.textContent)).toEqual(['Refunds', 'Billing', 'high', 'Yes', '2', 'Delete'])
  expect(screen.getByRole('link', { name: 'Refunds' })).toHaveAttribute('href', '/admin/topics/2')
  expect(screen.getByRole('link', { name: /new topic/i })).toHaveAttribute('href', '/admin/topics/new')
})

test('delete confirms, calls the endpoint, and a 409 shows the message', async () => {
  const deleted: string[] = []
  server.use(http.delete('/api/v1/topics/:id', ({ params }) => {
    deleted.push(String(params.id))
    server.use(http.get('/api/v1/topics', () => HttpResponse.json({ items: referenceFixtures.topics.filter((t) => t.id !== 2) })))
    return new HttpResponse(null, { status: 204 })
  }))
  renderWithProviders(<App />, { route: '/admin/topics' })
  await userEvent.click(await screen.findByRole('button', { name: 'Delete Refunds' }))
  await userEvent.click(screen.getByRole('button', { name: /confirm/i }))
  await waitFor(() => expect(screen.queryByRole('link', { name: 'Refunds' })).not.toBeInTheDocument())
  expect(deleted).toEqual(['2'])
  server.use(http.delete('/api/v1/topics/:id', () => HttpResponse.json({ error: { code: 'conflict', message: 'topic is referenced by tickets' } }, { status: 409 })))
  await userEvent.click(screen.getByRole('button', { name: 'Delete General Inquiry' }))
  await userEvent.click(screen.getByRole('button', { name: /confirm/i }))
  expect(await screen.findByRole('alert')).toHaveTextContent('topic is referenced by tickets')
  expect(screen.getByRole('link', { name: 'General Inquiry' })).toBeInTheDocument()
})

test('create submits all fields and returns to the list', async () => {
  let body: unknown
  server.use(http.post('/api/v1/topics', async ({ request }) => { body = await request.json(); return HttpResponse.json({ id: 9, ...(body as object) }, { status: 201 }) }))
  renderWithProviders(<App />, { route: '/admin/topics/new' })
  await userEvent.type(await screen.findByLabelText(/^name$/i), 'Outages')
  await userEvent.selectOptions(screen.getByLabelText(/default department/i), '1')
  await userEvent.selectOptions(screen.getByLabelText(/default priority/i), '3')
  await userEvent.click(screen.getByLabelText(/^active$/i))
  await userEvent.clear(screen.getByLabelText(/sort order/i))
  await userEvent.type(screen.getByLabelText(/sort order/i), '5')
  await userEvent.click(screen.getByRole('button', { name: /^create$/i }))
  await waitFor(() => expect(body).toEqual({ name: 'Outages', dept_id: 1, priority_id: 3, is_active: false, sort_order: 5 }))
  expect(await screen.findByRole('heading', { name: 'Topics' })).toBeInTheDocument()
})

test('edit seeds the form and sends only changed fields', async () => {
  let body: unknown
  server.use(http.patch('/api/v1/topics/2', async ({ request }) => { body = await request.json(); return HttpResponse.json({ ...referenceFixtures.topics[1], ...(body as object) }) }))
  renderWithProviders(<App />, { route: '/admin/topics/2' })
  expect(await screen.findByRole('heading', { name: 'Edit Refunds' })).toBeInTheDocument()
  expect(screen.getByLabelText(/^name$/i)).toHaveValue('Refunds')
  expect(screen.getByLabelText(/default department/i)).toHaveValue('2')
  expect(screen.getByLabelText(/default priority/i)).toHaveValue('3')
  expect(screen.getByLabelText(/^active$/i)).toBeChecked()
  expect(screen.getByLabelText(/sort order/i)).toHaveValue(2)
  await userEvent.selectOptions(screen.getByLabelText(/default department/i), '')
  await userEvent.click(screen.getByRole('button', { name: /^save$/i }))
  await waitFor(() => expect(body).toEqual({ dept_id: null }))
})

test('field errors render under the fields', async () => {
  server.use(http.post('/api/v1/topics', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { name: 'required', sort_order: 'must be >= 0' } } }, { status: 400 })))
  renderWithProviders(<App />, { route: '/admin/topics/new' })
  await userEvent.click(await screen.findByRole('button', { name: /^create$/i }))
  expect(await screen.findByText('required')).toBeInTheDocument()
  expect(screen.getByText('must be >= 0')).toBeInTheDocument()
})

test('unknown or invalid id renders not found', async () => {
  renderWithProviders(<App />, { route: '/admin/topics/abc' })
  expect(await screen.findByRole('heading', { name: /page not found/i })).toBeInTheDocument()
})

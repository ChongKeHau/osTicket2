import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../../App'
import { signInAsAdmin } from '../../test/admin'
import { referenceFixtures } from '../../test/fixtures'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/setup'

beforeEach(() => signInAsAdmin())

const rowOf = (name: string) => screen.getByRole('link', { name }).closest('tr')!
const bodyRows = () => within(screen.getAllByRole('rowgroup')[1]!).getAllByRole('row')

async function askDelete(name: string) {
  await userEvent.click(within(rowOf(name)).getByRole('button', { name: 'More' }))
  await userEvent.click(screen.getByRole('menuitem', { name: 'Delete' }))
}

test('list shows topics with department, priority, status, and sort order', async () => {
  renderWithProviders(<App />, { route: '/admin/topics' })
  expect(await screen.findByRole('link', { name: 'Refunds' })).toBeInTheDocument()
  expect(screen.getByRole('heading', { name: /^Help Topics/ })).toHaveTextContent('Help Topics (2)')
  const sub = screen.getByRole('navigation', { name: 'Secondary' })
  expect(within(sub).getByRole('link', { name: 'All Help Topics' })).toHaveAttribute('href', '/admin/topics')
  expect(within(sub).getByRole('link', { name: 'Add New Help Topic' })).toHaveAttribute('href', '/admin/topics/new')
  const rows = bodyRows()
  expect(rows).toHaveLength(2)
  expect(within(rows[1]!).getAllByRole('cell').map((c) => c.textContent)).toEqual(['Refunds', 'Billing', 'high', 'Active', '2', 'More ▾'])
  expect(screen.getByRole('link', { name: 'Refunds' })).toHaveAttribute('href', '/admin/topics/2')
})

test('list sorts client-side by sort order and name', async () => {
  renderWithProviders(<App />, { route: '/admin/topics' })
  await screen.findByRole('link', { name: 'Refunds' })
  const first = () => within(bodyRows()[0]!).getAllByRole('cell')[0]!.textContent
  expect(first()).toBe('General Inquiry')
  await userEvent.click(screen.getByRole('button', { name: /Sort order/ }))
  expect(first()).toBe('Refunds')
  await userEvent.click(screen.getByRole('button', { name: /Name/ }))
  expect(first()).toBe('General Inquiry')
})

test('delete confirms from the More menu, calls the endpoint, and a 409 shows the message', async () => {
  const deleted: string[] = []
  server.use(http.delete('/api/v1/topics/:id', ({ params }) => {
    deleted.push(String(params.id))
    server.use(http.get('/api/v1/topics', () => HttpResponse.json({ items: referenceFixtures.topics.filter((t) => t.id !== 2) })))
    return new HttpResponse(null, { status: 204 })
  }))
  renderWithProviders(<App />, { route: '/admin/topics' })
  await screen.findByRole('link', { name: 'Refunds' })
  await askDelete('Refunds')
  await userEvent.click(screen.getByRole('button', { name: 'Confirm delete Refunds' }))
  await waitFor(() => expect(screen.queryByRole('link', { name: 'Refunds' })).not.toBeInTheDocument())
  expect(deleted).toEqual(['2'])
  expect(screen.getByRole('status')).toHaveTextContent('Help topic "Refunds" deleted')
  server.use(http.delete('/api/v1/topics/:id', () => HttpResponse.json({ error: { code: 'conflict', message: 'topic is referenced by tickets' } }, { status: 409 })))
  await askDelete('General Inquiry')
  await userEvent.click(screen.getByRole('button', { name: 'Confirm delete General Inquiry' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('topic is referenced by tickets')
  expect(screen.getByRole('link', { name: 'General Inquiry' })).toBeInTheDocument()
})

test('create submits all fields, flashes, and returns to the list', async () => {
  let body: unknown
  server.use(http.post('/api/v1/topics', async ({ request }) => { body = await request.json(); return HttpResponse.json({ id: 9, ...(body as object) }, { status: 201 }) }))
  renderWithProviders(<App />, { route: '/admin/topics/new' })
  expect(await screen.findByRole('heading', { name: 'Add New Help Topic' })).toBeInTheDocument()
  expect(screen.getByText('Settings')).toBeInTheDocument()
  expect(screen.getByText('Routing')).toBeInTheDocument()
  await userEvent.type(screen.getByLabelText(/^Name/), 'Outages')
  await userEvent.selectOptions(screen.getByLabelText(/^Department/), '1')
  await userEvent.selectOptions(screen.getByLabelText(/^Priority/), '3')
  await userEvent.click(screen.getByLabelText(/^Active/))
  await userEvent.clear(screen.getByLabelText(/^Sort order/))
  await userEvent.type(screen.getByLabelText(/^Sort order/), '5')
  await userEvent.click(screen.getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(body).toEqual({ name: 'Outages', dept_id: 1, priority_id: 3, is_active: false, sort_order: 5 }))
  expect(await screen.findByRole('heading', { name: /^Help Topics/ })).toBeInTheDocument()
  expect(screen.getByRole('status')).toHaveTextContent('Help topic saved')
})

test('a negative sort order is sent as typed', async () => {
  let body: unknown
  server.use(http.post('/api/v1/topics', async ({ request }) => { body = await request.json(); return HttpResponse.json({ id: 9, ...(body as object) }, { status: 201 }) }))
  renderWithProviders(<App />, { route: '/admin/topics/new' })
  await userEvent.type(await screen.findByLabelText(/^Name/), 'Outages')
  await userEvent.clear(screen.getByLabelText(/^Sort order/))
  await userEvent.type(screen.getByLabelText(/^Sort order/), '-5')
  await userEvent.click(screen.getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(body).toMatchObject({ sort_order: -5 }))
})

test('an empty sort order is sent as 0', async () => {
  let body: unknown
  server.use(http.patch('/api/v1/topics/2', async ({ request }) => { body = await request.json(); return HttpResponse.json({ ...referenceFixtures.topics[1], ...(body as object) }) }))
  renderWithProviders(<App />, { route: '/admin/topics/2' })
  await userEvent.clear(await screen.findByLabelText(/^Sort order/))
  expect(screen.getByLabelText(/^Sort order/)).toHaveValue(null)
  await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() => expect(body).toEqual({ sort_order: 0 }))
})

test('edit seeds the form, sends only changed fields, and stays on the form', async () => {
  let body: unknown
  server.use(http.patch('/api/v1/topics/2', async ({ request }) => { body = await request.json(); return HttpResponse.json({ ...referenceFixtures.topics[1], ...(body as object) }) }))
  renderWithProviders(<App />, { route: '/admin/topics/2' })
  expect(await screen.findByRole('heading', { name: 'Help Topic: Refunds' })).toBeInTheDocument()
  expect(screen.getByLabelText(/^Name/)).toHaveValue('Refunds')
  expect(screen.getByLabelText(/^Department/)).toHaveValue('2')
  expect(screen.getByLabelText(/^Priority/)).toHaveValue('3')
  expect(screen.getByLabelText(/^Active/)).toBeChecked()
  expect(screen.getByLabelText(/^Sort order/)).toHaveValue(2)
  await userEvent.selectOptions(screen.getByLabelText(/^Department/), '')
  await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() => expect(body).toEqual({ dept_id: null }))
  expect(await screen.findByRole('status')).toHaveTextContent('Help topic saved')
  expect(screen.getByRole('heading', { name: 'Help Topic: Refunds' })).toBeInTheDocument()
})

test('reset restores the loaded values, including the sort order', async () => {
  renderWithProviders(<App />, { route: '/admin/topics/2' })
  const name = await screen.findByLabelText(/^Name/)
  await userEvent.clear(name)
  await userEvent.type(name, 'Changed')
  await userEvent.clear(screen.getByLabelText(/^Sort order/))
  await userEvent.type(screen.getByLabelText(/^Sort order/), '9')
  await userEvent.click(screen.getByRole('button', { name: 'Reset' }))
  expect(name).toHaveValue('Refunds')
  expect(screen.getByLabelText(/^Sort order/)).toHaveValue(2)
})

test('field errors render in their rows', async () => {
  server.use(http.post('/api/v1/topics', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { name: 'required', sort_order: 'must be >= 0' } } }, { status: 400 })))
  renderWithProviders(<App />, { route: '/admin/topics/new' })
  await userEvent.click(await screen.findByRole('button', { name: 'Create' }))
  expect(await screen.findByText('required')).toBeInTheDocument()
  expect(screen.getByText('must be >= 0')).toBeInTheDocument()
  expect(screen.getByLabelText(/^Name/)).toHaveAttribute('aria-invalid', 'true')
})

test('unknown or invalid id renders not found', async () => {
  renderWithProviders(<App />, { route: '/admin/topics/abc' })
  expect(await screen.findByRole('heading', { name: /page not found/i })).toBeInTheDocument()
})

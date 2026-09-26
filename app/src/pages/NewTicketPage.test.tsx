import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../App'
import { REFRESH_KEY } from '../api/client'
import { referenceFixtures, staffProfileFixture, ticketFixture } from '../test/fixtures'
import { renderWithProviders } from '../test/render'
import { server } from '../test/setup'

beforeEach(() => localStorage.setItem(REFRESH_KEY, 'refresh-1'))

function mount() {
  return renderWithProviders(<App />, { route: '/tickets/new' })
}

test('topic pre-fills department and priority; submit navigates to the new ticket', async () => {
  let body: unknown
  server.use(http.post('/api/v1/tickets', async ({ request }) => { body = await request.json(); return HttpResponse.json({ ...ticketFixture, id: 8, number: '000008', subject: 'Refund please' }, { status: 201 }) }))
  server.use(http.get('/api/v1/tickets/8', () => HttpResponse.json({ ...ticketFixture, id: 8, number: '000008', subject: 'Refund please' })))
  server.use(http.get('/api/v1/me', () => HttpResponse.json({ ...staffProfileFixture, department_ids: [1, 2] })))
  mount()
  await userEvent.selectOptions(await screen.findByLabelText(/help topic/i), '2')
  expect(screen.getByLabelText(/^department$/i)).toHaveValue('2')
  expect(screen.getByLabelText(/^priority$/i)).toHaveValue('3')
  await userEvent.type(screen.getByLabelText(/^subject/i), 'Refund please')
  await userEvent.type(screen.getByLabelText(/^message/i), 'I was charged twice')
  await userEvent.type(screen.getByLabelText(/^name$/i), 'Pat')
  await userEvent.type(screen.getByLabelText(/^email/i), 'pat@example.test')
  await userEvent.selectOptions(screen.getByLabelText(/^source$/i), 'phone')
  await userEvent.click(screen.getByRole('button', { name: /open ticket/i }))
  await waitFor(() => expect(body).toEqual({
    subject: 'Refund please', message: 'I was charged twice', message_format: 'text', requester_name: 'Pat', requester_email: 'pat@example.test',
    topic_id: 2, dept_id: 2, priority_id: 3, source: 'phone', file_ids: [],
  }))
  expect(await screen.findByRole('heading', { name: /000008/ })).toBeInTheDocument()
})

test('department lists only departments the agent can use; an unusable topic default is ignored', async () => {
  // Default profile: non-admin, department_ids [1].
  mount()
  const dept = await screen.findByLabelText(/^department$/i)
  expect(within(dept).getAllByRole('option').map((o) => o.textContent)).toEqual(['Choose…', 'Support'])
  await userEvent.selectOptions(await screen.findByLabelText(/help topic/i), '2') // Refunds defaults to Billing (2)
  expect(dept).toHaveValue('0')
  expect(screen.getByLabelText(/^priority$/i)).toHaveValue('3')
  await userEvent.selectOptions(screen.getByLabelText(/help topic/i), '1') // General Inquiry defaults to Support (1)
  expect(dept).toHaveValue('1')
})

test('validation errors map to fields', async () => {
  server.use(http.post('/api/v1/tickets', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { requester_email: 'email', dept_id: 'required when no topic supplies a department' } } }, { status: 400 })))
  mount()
  await userEvent.type(await screen.findByLabelText(/^subject/i), 's')
  await userEvent.type(screen.getByLabelText(/^message/i), 'm')
  await userEvent.type(screen.getByLabelText(/^email/i), 'a@b.test')
  await userEvent.click(screen.getByRole('button', { name: /open ticket/i }))
  expect(await screen.findByText(/required when no topic/i)).toBeInTheDocument()
  expect(screen.getByText('email')).toBeInTheDocument()
})

test('a source validation error renders under the Source select', async () => {
  server.use(http.post('/api/v1/tickets', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { source: 'invalid source' } } }, { status: 422 })))
  mount()
  await userEvent.type(await screen.findByLabelText(/^subject/i), 's')
  await userEvent.type(screen.getByLabelText(/^message/i), 'm')
  await userEvent.type(screen.getByLabelText(/^email/i), 'a@b.test')
  await userEvent.click(screen.getByRole('button', { name: /open ticket/i }))
  const source = await screen.findByLabelText(/^source$/i)
  expect(within(source.closest('tr') as HTMLElement).getByText('invalid source')).toBeInTheDocument()
})

test('a /staff failure (data this page never uses) does not block the form', async () => {
  server.use(http.get('/api/v1/staff', () => HttpResponse.json({ error: { code: 'server_error', message: 'boom' } }, { status: 500 })))
  mount()
  expect(await screen.findByLabelText(/help topic/i)).toBeInTheDocument()
})

test('a /topics failure shows a retryable banner; retry recovers the form', async () => {
  server.use(http.get('/api/v1/topics', () => HttpResponse.json({ error: { code: 'server_error', message: 'boom' } }, { status: 500 })))
  mount()
  const retry = await screen.findByRole('button', { name: /retry/i })
  server.use(http.get('/api/v1/topics', () => HttpResponse.json({ items: referenceFixtures.topics })))
  await userEvent.click(retry)
  expect(await screen.findByLabelText(/help topic/i)).toBeInTheDocument()
})

it('lays the form out in two sections with required markers', async () => {
  mount()
  expect(await screen.findByText('User Information')).toBeInTheDocument()
  expect(screen.getByText('Ticket Details')).toBeInTheDocument()
  expect(screen.getByLabelText(/Email/)).toBeRequired()
  expect(screen.getByLabelText(/Subject/)).toBeRequired()
  expect(screen.getByLabelText(/Source/)).toHaveValue('phone')
})

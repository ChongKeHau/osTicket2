import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../App'
import { REFRESH_KEY } from '../api/client'
import { ticketFixture } from '../test/fixtures'
import { renderWithProviders } from '../test/render'
import { server } from '../test/setup'

beforeEach(() => localStorage.setItem(REFRESH_KEY, 'refresh-1'))

test('topic pre-fills department and priority; submit navigates to the new ticket', async () => {
  let body: unknown
  server.use(http.post('/api/v1/tickets', async ({ request }) => { body = await request.json(); return HttpResponse.json({ ...ticketFixture, id: 8, number: '000008', subject: 'Refund please' }, { status: 201 }) }))
  server.use(http.get('/api/v1/tickets/8', () => HttpResponse.json({ ...ticketFixture, id: 8, number: '000008', subject: 'Refund please' })))
  renderWithProviders(<App />, { route: '/tickets/new' })
  await userEvent.selectOptions(await screen.findByLabelText(/^topic$/i), '2')
  expect(screen.getByLabelText(/^department$/i)).toHaveValue('2')
  expect(screen.getByLabelText(/^priority$/i)).toHaveValue('3')
  await userEvent.type(screen.getByLabelText(/^subject$/i), 'Refund please')
  await userEvent.type(screen.getByLabelText(/^message$/i), 'I was charged twice')
  await userEvent.type(screen.getByLabelText(/requester name/i), 'Pat')
  await userEvent.type(screen.getByLabelText(/requester email/i), 'pat@example.test')
  await userEvent.selectOptions(screen.getByLabelText(/^source$/i), 'phone')
  await userEvent.click(screen.getByRole('button', { name: /create ticket/i }))
  await waitFor(() => expect(body).toEqual({
    subject: 'Refund please', message: 'I was charged twice', message_format: 'text', requester_name: 'Pat', requester_email: 'pat@example.test',
    topic_id: 2, dept_id: 2, priority_id: 3, source: 'phone', file_ids: [],
  }))
  expect(await screen.findByRole('heading', { name: /000008/ })).toBeInTheDocument()
})

test('validation errors map to fields', async () => {
  server.use(http.post('/api/v1/tickets', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { requester_email: 'email', dept_id: 'required when no topic supplies a department' } } }, { status: 400 })))
  renderWithProviders(<App />, { route: '/tickets/new' })
  await userEvent.type(await screen.findByLabelText(/^subject$/i), 's')
  await userEvent.type(screen.getByLabelText(/^message$/i), 'm')
  await userEvent.type(screen.getByLabelText(/requester email/i), 'a@b.test')
  await userEvent.click(screen.getByRole('button', { name: /create ticket/i }))
  expect(await screen.findByText(/required when no topic/i)).toBeInTheDocument()
  expect(screen.getByText('email')).toBeInTheDocument()
})

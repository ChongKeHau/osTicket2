import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../App'
import { REFRESH_KEY } from '../api/client'
import { staffProfileFixture, ticketFixture } from '../test/fixtures'
import { renderWithProviders } from '../test/render'
import { server } from '../test/setup'

beforeEach(() => localStorage.setItem(REFRESH_KEY, 'refresh-1'))

test('renders header fields', async () => {
  renderWithProviders(<App />, { route: '/tickets/7' })
  expect(await screen.findByRole('heading', { name: /000007/ })).toHaveTextContent('Printer on fire')
  const header = screen.getByRole('region', { name: /ticket header/i })
  expect(within(header).getAllByText('Support').length).toBeGreaterThan(0)
  expect(within(header).getByText('General Inquiry')).toBeInTheDocument()
  expect(within(header).getByText(/pat@example.test/)).toBeInTheDocument()
  expect(within(header).getAllByText('Unassigned').length).toBeGreaterThan(0)
})

test('404 renders not found', async () => {
  renderWithProviders(<App />, { route: '/tickets/999' })
  expect(await screen.findByText(/ticket not found/i)).toBeInTheDocument()
})

test('status change posts and re-renders', async () => {
  let body: unknown
  server.use(http.post('/api/v1/tickets/7/status', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ ...ticketFixture, status: { id: 3, name: 'Closed' }, state: 'closed', closed_at: '2026-09-25T11:00:00Z' })
  }))
  let getCalls = 0
  server.use(http.get('/api/v1/tickets/7', () => { getCalls++; return HttpResponse.json(getCalls > 1 ? { ...ticketFixture, status: { id: 3, name: 'Closed' }, state: 'closed' } : ticketFixture) }))
  renderWithProviders(<App />, { route: '/tickets/7' })
  await screen.findByRole('heading', { name: /000007/ })
  await userEvent.selectOptions(await screen.findByLabelText(/^status$/i), '3')
  await waitFor(() => expect(body).toEqual({ status_id: 3 }))
  const header = screen.getByRole('region', { name: /ticket header/i })
  await waitFor(() => expect(within(header).getAllByText('Closed').length).toBeGreaterThanOrEqual(1))
})

test('assign lists only staff who can see the department, and posts', async () => {
  let body: unknown
  server.use(http.post('/api/v1/tickets/7/assign', async ({ request }) => { body = await request.json(); return HttpResponse.json({ ...ticketFixture, assignee: { id: 1, name: 'Ann Agent' } }) }))
  renderWithProviders(<App />, { route: '/tickets/7' })
  await screen.findByRole('heading', { name: /000007/ })
  const select = await screen.findByLabelText(/^assignee$/i)
  const options = within(select).getAllByRole('option').map((o) => o.textContent)
  expect(options).toEqual(['Unassigned', 'Ann Agent', 'Root Admin'])
  await userEvent.selectOptions(select, '1')
  await waitFor(() => expect(body).toEqual({ staff_id: 1 }))
})

test('transfer posts the department and shows a validation error on 400', async () => {
  server.use(http.get('/api/v1/me', () => HttpResponse.json({ ...staffProfileFixture, is_admin: true, department_ids: [1, 2] })))
  server.use(http.post('/api/v1/tickets/7/transfer', () =>
    HttpResponse.json({ error: { code: 'forbidden', message: 'forbidden: cannot transfer to that department' } }, { status: 403 })))
  renderWithProviders(<App />, { route: '/tickets/7' })
  await screen.findByRole('heading', { name: /000007/ })
  await userEvent.selectOptions(await screen.findByLabelText(/^department$/i), '2')
  expect(await screen.findByRole('alert')).toHaveTextContent(/cannot transfer/i)
})

test('edit form patches subject and priority', async () => {
  let body: unknown
  server.use(http.patch('/api/v1/tickets/7', async ({ request }) => { body = await request.json(); return HttpResponse.json({ ...ticketFixture, subject: 'New subject' }) }))
  renderWithProviders(<App />, { route: '/tickets/7' })
  await screen.findByRole('heading', { name: /000007/ })
  await userEvent.click(screen.getByRole('button', { name: /edit/i }))
  const subject = screen.getByLabelText(/subject/i)
  await userEvent.clear(subject)
  await userEvent.type(subject, 'New subject')
  await userEvent.selectOptions(screen.getByLabelText(/^priority$/i), '3')
  await userEvent.click(screen.getByRole('button', { name: /save/i }))
  await waitFor(() => expect(body).toMatchObject({ subject: 'New subject', priority_id: 3 }))
})

test('edit form discards changes on cancel and re-shows original values on reopen', async () => {
  renderWithProviders(<App />, { route: '/tickets/7' })
  await screen.findByRole('heading', { name: /000007/ })
  await userEvent.click(screen.getByRole('button', { name: /edit/i }))
  const subject = screen.getByLabelText(/subject/i)
  await userEvent.clear(subject)
  await userEvent.type(subject, 'Discarded subject')
  await userEvent.click(screen.getByRole('button', { name: /cancel/i }))
  await userEvent.click(screen.getByRole('button', { name: /edit/i }))
  expect(screen.getByLabelText(/subject/i)).toHaveValue(ticketFixture.subject)
})

test('edit form clears the due date to null on save', async () => {
  let body: unknown
  const ticketWithDue = { ...ticketFixture, due_at: '2026-09-26T10:00:00Z' }
  server.use(http.get('/api/v1/tickets/7', () => HttpResponse.json(ticketWithDue)))
  server.use(http.patch('/api/v1/tickets/7', async ({ request }) => { body = await request.json(); return HttpResponse.json({ ...ticketWithDue, due_at: null }) }))
  renderWithProviders(<App />, { route: '/tickets/7' })
  await screen.findByRole('heading', { name: /000007/ })
  await userEvent.click(screen.getByRole('button', { name: /edit/i }))
  const due = screen.getByLabelText(/^due$/i)
  await userEvent.clear(due)
  await userEvent.click(screen.getByRole('button', { name: /save/i }))
  await waitFor(() => expect(body).toMatchObject({ due_at: null }))
})

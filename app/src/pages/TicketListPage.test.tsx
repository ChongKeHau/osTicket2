import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../App'
import { REFRESH_KEY } from '../api/client'
import { ticketFixture } from '../test/fixtures'
import { renderWithProviders } from '../test/render'
import { server } from '../test/setup'

let lastQuery = new URLSearchParams()
beforeEach(() => {
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  lastQuery = new URLSearchParams()
  server.use(http.get('/api/v1/tickets', ({ request }) => {
    lastQuery = new URL(request.url).searchParams
    const page = Number(lastQuery.get('page') ?? '1')
    return HttpResponse.json({ items: page === 1 ? [ticketFixture] : [{ ...ticketFixture, id: 8, number: '000008', subject: 'Second' }], page, page_size: 1, total: 2 })
  }))
})

test('renders rows and links to the detail page', async () => {
  renderWithProviders(<App />, { route: '/tickets' })
  expect(await screen.findByText('Printer on fire')).toBeInTheDocument()
  expect(screen.getByRole('link', { name: /000007/ })).toHaveAttribute('href', '/tickets/7')
  expect(within(screen.getByRole('table')).getByText('Open')).toBeInTheDocument()
})

test('filters update the request query and the URL', async () => {
  renderWithProviders(<App />, { route: '/tickets' })
  await screen.findByText('Printer on fire')
  await userEvent.selectOptions(screen.getByLabelText(/state/i), 'closed')
  await waitFor(() => expect(lastQuery.get('state')).toBe('closed'))
  await userEvent.selectOptions(screen.getByLabelText(/assigned/i), 'me')
  await waitFor(() => expect(lastQuery.get('assigned_to')).toBe('me'))
  await userEvent.type(screen.getByLabelText(/search/i), 'printer')
  await waitFor(() => expect(lastQuery.get('q')).toBe('printer'), { timeout: 2000 })
  expect(lastQuery.get('page')).toBe('1')
})

test('sort header toggles direction', async () => {
  renderWithProviders(<App />, { route: '/tickets' })
  await screen.findByText('Printer on fire')
  await userEvent.click(screen.getByRole('button', { name: /priority/i }))
  await waitFor(() => expect(lastQuery.get('sort')).toBe('priority'))
  await userEvent.click(screen.getByRole('button', { name: /priority/i }))
  await waitFor(() => expect(lastQuery.get('sort')).toBe('-priority'))
})

test('pagination requests the next page', async () => {
  renderWithProviders(<App />, { route: '/tickets?page_size=1' })
  await screen.findByText('Printer on fire')
  await userEvent.click(screen.getByRole('button', { name: /next/i }))
  expect(await screen.findByText('Second')).toBeInTheDocument()
  expect(lastQuery.get('page')).toBe('2')
})

test('shows the error banner with retry', async () => {
  server.use(http.get('/api/v1/tickets', () => HttpResponse.json({ error: { code: 'internal', message: 'internal server error' } }, { status: 500 })))
  renderWithProviders(<App />, { route: '/tickets' })
  expect(await screen.findByRole('alert')).toHaveTextContent(/internal server error/i)
  expect(screen.getByRole('button', { name: /retry/i })).toBeInTheDocument()
})

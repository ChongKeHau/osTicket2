import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import App from '../App'
import { REFRESH_KEY } from '../api/client'
import { dashboardFixture } from '../test/fixtures'
import { renderWithProviders } from '../test/render'
import { server } from '../test/setup'

function mount(route: string) {
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  return renderWithProviders(<App />, { route })
}

const daysAgo = (n: number) => new Date(Date.now() - n * 86400000).toISOString().slice(0, 10)

it('renders the period form, chart and statistics tabs', async () => {
  mount('/dashboard')
  expect(await screen.findByRole('heading', { name: 'Ticket Activity' })).toBeInTheDocument()
  expect(screen.getByLabelText('Period')).toHaveValue('30')
  expect(await screen.findByRole('img', { name: /Opened, Assigned, Closed, Reopened/ })).toBeInTheDocument()
  expect(screen.getByRole('tab', { name: 'Department' })).toHaveAttribute('aria-selected', 'true')
  expect(screen.getAllByRole('row')).toHaveLength(1 + dashboardFixture.by_department.length + 1) // header + rows + totals
  const totalCells = screen.getAllByRole('row').at(-1)!.querySelectorAll('td')
  expect(totalCells[0]).toHaveTextContent('Total')
  expect(totalCells[0]).toHaveClass('total')
  expect(totalCells[1]).toHaveTextContent(String(dashboardFixture.by_department.reduce((n, r) => n + r.opened, 0)))
  await userEvent.click(screen.getByRole('tab', { name: 'Help Topic' }))
  expect(screen.getByText('— none —')).toBeInTheDocument()
})

it('defaults to a window that ends today and follows the period until refreshed', async () => {
  const seen: string[] = []
  server.use(http.get('/api/v1/dashboard/stats', ({ request }) => { seen.push(new URL(request.url).search); return HttpResponse.json(dashboardFixture) }))
  mount('/dashboard')
  await screen.findByRole('heading', { name: 'Ticket Activity' })
  expect(screen.getByLabelText('Start')).toHaveValue(daysAgo(29))
  await waitFor(() => expect(seen[0]).toBe(`?start=${daysAgo(29)}&period=30`))
  await userEvent.selectOptions(screen.getByLabelText('Period'), '7')
  expect(screen.getByLabelText('Start')).toHaveValue(daysAgo(6))
})

it('refetches with the chosen start and period', async () => {
  const seen: string[] = []
  server.use(http.get('/api/v1/dashboard/stats', ({ request }) => { seen.push(new URL(request.url).search); return HttpResponse.json(dashboardFixture) }))
  mount('/dashboard')
  await screen.findByRole('heading', { name: 'Ticket Activity' })
  await userEvent.selectOptions(screen.getByLabelText('Period'), '7')
  await userEvent.clear(screen.getByLabelText('Start')); await userEvent.type(screen.getByLabelText('Start'), '2026-09-01')
  await userEvent.click(screen.getByRole('button', { name: 'Refresh' }))
  await waitFor(() => expect(seen.at(-1)).toBe('?start=2026-09-01&period=7'))
})

it('refetches on Refresh even when start and period are unchanged', async () => {
  const seen: string[] = []
  server.use(http.get('/api/v1/dashboard/stats', ({ request }) => { seen.push(new URL(request.url).search); return HttpResponse.json(dashboardFixture) }))
  mount('/dashboard')
  await screen.findByRole('img', { name: /Opened/ })
  expect(seen).toHaveLength(1)
  await userEvent.click(screen.getByRole('button', { name: 'Refresh' }))
  await waitFor(() => expect(seen).toHaveLength(2))
  await userEvent.click(screen.getByRole('button', { name: 'Refresh' }))
  await waitFor(() => expect(seen).toHaveLength(3))
  expect(new Set(seen).size).toBe(1)
})

it('shows only the error banner when the request fails', async () => {
  server.use(http.get('/api/v1/dashboard/stats', () => HttpResponse.json({ error: { code: 'bad_request', message: 'start must be a date' } }, { status: 400 })))
  mount('/dashboard')
  expect(await screen.findByRole('alert')).toHaveTextContent('start must be a date')
  expect(screen.queryByText('No activity in this period')).not.toBeInTheDocument()
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
  expect(screen.queryByText('Loading…')).not.toBeInTheDocument()
})

it('shows a loading line instead of the table while the first request is pending', async () => {
  server.use(http.get('/api/v1/dashboard/stats', () => new Promise(() => {})))
  mount('/dashboard')
  expect(await screen.findByText('Loading…', { selector: 'p' })).toBeInTheDocument()
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
  expect(screen.queryByText('No activity in this period')).not.toBeInTheDocument()
})

it('shows the empty state when every count is zero', async () => {
  server.use(http.get('/api/v1/dashboard/stats', () => HttpResponse.json({ ...dashboardFixture, series: dashboardFixture.series.map((p) => ({ ...p, opened: 0, assigned: 0, closed: 0, reopened: 0 })), by_department: [], by_topic: [], by_staff: [] })))
  mount('/dashboard')
  expect(await screen.findByText('No activity in this period', { selector: 'p' })).toBeInTheDocument()
})

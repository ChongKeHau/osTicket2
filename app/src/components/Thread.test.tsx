import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { REFRESH_KEY, tokens } from '../api/client'
import { sessionFixture, entryFixtures } from '../test/fixtures'
import { renderWithProviders } from '../test/render'
import { server } from '../test/setup'
import { EventsPanel } from './EventsPanel'
import { Thread } from './Thread'

beforeEach(() => { localStorage.setItem(REFRESH_KEY, 'refresh-1'); tokens.setSession(sessionFixture) })

test('renders entries oldest first with type badges and attachments', async () => {
  renderWithProviders(<Thread ticketId={7} />)
  const items = await screen.findAllByRole('article')
  expect(items).toHaveLength(2)
  expect(within(items[0]!).getByText('Message')).toBeInTheDocument()
  expect(within(items[0]!).getByText('Pat')).toBeInTheDocument()
  expect(within(items[1]!).getByText('Response')).toBeInTheDocument()
  expect(within(items[1]!).getByRole('button', { name: /log\.txt/ })).toBeInTheDocument()
})

test('sanitises HTML bodies', async () => {
  server.use(http.get('/api/v1/tickets/7/thread', () => HttpResponse.json({
    items: [{ ...entryFixtures[0], body: '<p>hi</p><script>window.__pwned=1</script><img src=x onerror="window.__pwned=2">' }], next_after: null,
  })))
  renderWithProviders(<Thread ticketId={7} />)
  const item = await screen.findByRole('article')
  expect(item.querySelector('script')).toBeNull()
  expect(item.querySelector('img')?.getAttribute('onerror')).toBeNull()
  expect(item).toHaveTextContent('hi')
  expect((window as unknown as { __pwned?: number }).__pwned).toBeUndefined()
})

test('text bodies keep line breaks and are not parsed as HTML', async () => {
  server.use(http.get('/api/v1/tickets/7/thread', () => HttpResponse.json({
    items: [{ ...entryFixtures[1], body: 'line one\n<b>not bold</b>' }], next_after: null,
  })))
  renderWithProviders(<Thread ticketId={7} />)
  const item = await screen.findByRole('article')
  expect(item.querySelector('b')).toBeNull()
  expect(item).toHaveTextContent('<b>not bold</b>')
})

test('load more appends the next page', async () => {
  server.use(http.get('/api/v1/tickets/7/thread', ({ request }) => {
    const after = new URL(request.url).searchParams.get('after')
    if (!after) return HttpResponse.json({ items: [entryFixtures[0]], next_after: 1 })
    return HttpResponse.json({ items: [entryFixtures[1]], next_after: null })
  }))
  renderWithProviders(<Thread ticketId={7} />)
  expect(await screen.findAllByRole('article')).toHaveLength(1)
  await userEvent.click(screen.getByRole('button', { name: /load more/i }))
  expect(await screen.findAllByRole('article')).toHaveLength(2)
  expect(screen.queryByRole('button', { name: /load more/i })).not.toBeInTheDocument()
})

test('attachment click downloads with auth', async () => {
  const createObjectURL = vi.fn(() => 'blob:x')
  Object.assign(URL, { createObjectURL, revokeObjectURL: vi.fn() })
  const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  renderWithProviders(<Thread ticketId={7} />)
  await userEvent.click(await screen.findByRole('button', { name: /log\.txt/ }))
  await vi.waitFor(() => expect(click).toHaveBeenCalledTimes(1))
  click.mockRestore()
})

test('events panel lists the audit trail when expanded', async () => {
  renderWithProviders(<EventsPanel ticketId={7} />)
  await userEvent.click(screen.getByRole('button', { name: /history/i }))
  expect(await screen.findByText(/created/i)).toBeInTheDocument()
  expect(screen.getByText(/Ann Agent/)).toBeInTheDocument()
})

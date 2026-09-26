import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { REFRESH_KEY } from '../api/client'
import { AuthProvider } from '../auth/AuthContext'
import { RequireAuth } from '../auth/RequireAuth'
import { entryFixtures, eventFixtures, ticketFixture } from '../test/fixtures'
import { renderWithProviders } from '../test/render'
import { AppShell } from '../ui/AppShell'
import { TicketDetailPage } from './TicketDetailPage'

function mount(route: string) {
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  return renderWithProviders(
    <AuthProvider><Routes><Route element={<RequireAuth />}><Route element={<AppShell panel="agent" />}>
      <Route path="/tickets/:id" element={<TicketDetailPage />} />
    </Route></Route></Routes></AuthProvider>,
    { route },
  )
}

it('renders the sticky bar, subject, info tables and thread', async () => {
  mount(`/tickets/${ticketFixture.id}`)
  expect(await screen.findByRole('heading', { name: `Ticket #${ticketFixture.number}` })).toBeInTheDocument()
  expect(screen.getByRole('heading', { name: ticketFixture.subject })).toBeInTheDocument()
  expect(screen.getByText('Help Topic', { selector: 'th' })).toBeInTheDocument()
  const thread = await screen.findByRole('region', { name: 'Thread' })
  for (const e of entryFixtures) expect(within(thread).getAllByText(e.poster).length).toBeGreaterThan(0)
  expect(screen.getByRole('link', { name: 'New Ticket' })).toHaveAttribute('href', '/tickets/new')
  expect(document.title).toBe(`#${ticketFixture.number} – ${ticketFixture.subject}`)
})

it('keeps History collapsed until it is opened', async () => {
  mount(`/tickets/${ticketFixture.id}`)
  const history = await screen.findByRole('group', { name: 'History' })
  const kind = eventFixtures[0]!.kind
  expect(within(history).queryByText(kind, { exact: false })).not.toBeInTheDocument()
  await userEvent.click(within(history).getByText('History'))
  expect(await within(history).findByText(kind, { exact: false })).toBeInTheDocument()
})

it('Post Note selects the Internal Note tab', async () => {
  mount(`/tickets/${ticketFixture.id}`)
  await screen.findByRole('heading', { name: `Ticket #${ticketFixture.number}` })
  expect(screen.getByRole('tab', { name: 'Reply' })).toHaveAttribute('aria-selected', 'true')
  await userEvent.click(screen.getByRole('button', { name: 'Post Note' }))
  expect(screen.getByRole('tab', { name: 'Internal Note' })).toHaveAttribute('aria-selected', 'true')
  expect(screen.getByLabelText('Note')).toBeInTheDocument()
})

it('renders the not-found banner for an unknown id', async () => {
  mount('/tickets/999')
  expect(await screen.findByRole('alert')).toHaveTextContent('Ticket not found.')
})

it('renders the not-found banner for a malformed id', async () => {
  mount('/tickets/abc')
  expect(await screen.findByRole('alert')).toHaveTextContent('Ticket not found.')
})

import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { server } from '../test/setup'
import { adminProfileFixture, referenceFixtures, ticketFixture } from '../test/fixtures'
import { renderAuthed } from '../test/render'
import { TicketInfo } from './TicketInfo'

it('renders both tables and edits priority inline', async () => {
  let body: unknown
  server.use(http.patch('/api/v1/tickets/:id', async ({ request }) => { body = await request.json(); return HttpResponse.json(ticketFixture) }))
  renderAuthed(<TicketInfo ticket={ticketFixture} />)
  await screen.findByText('Status', { selector: 'th' })
  for (const label of ['Status', 'Priority', 'Department', 'Created', 'User', 'Email', 'Source', 'Assigned To', 'Help Topic', 'Last Updated']) {
    expect(screen.getByText(label, { selector: 'th' })).toBeInTheDocument()
  }
  expect(screen.getByRole('link', { name: ticketFixture.requester_email })).toHaveAttribute('href', `mailto:${ticketFixture.requester_email}`)
  await userEvent.click(screen.getByRole('button', { name: new RegExp(`Priority: ${ticketFixture.priority.name}`) }))
  const other = referenceFixtures.priorities.find((p) => p.id !== ticketFixture.priority.id)!
  await userEvent.selectOptions(await screen.findByLabelText('Priority editor'), String(other.id))
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(body).toEqual({ priority_id: other.id }))
})

it('restores the value and shows the error when an inline edit fails', async () => {
  server.use(http.post('/api/v1/tickets/:id/status', () => HttpResponse.json({ error: { code: 'conflict', message: 'Ticket is locked' } }, { status: 409 })))
  renderAuthed(<TicketInfo ticket={ticketFixture} />)
  await userEvent.click(await screen.findByRole('button', { name: new RegExp(`Status: ${ticketFixture.status.name}`) }))
  const other = referenceFixtures.statuses.find((st) => st.id !== ticketFixture.status.id)!
  await userEvent.selectOptions(await screen.findByLabelText('Status editor'), String(other.id))
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Ticket is locked')
  await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(screen.getByRole('button', { name: `Status: ${ticketFixture.status.name}` })).toBeInTheDocument()
})

it('edits subject, help topic, assignee, department, requester and due date', async () => {
  const bodies: string[] = []
  server.use(
    http.patch('/api/v1/tickets/:id', async ({ request }) => { bodies.push(`patch ${JSON.stringify(await request.json())}`); return HttpResponse.json(ticketFixture) }),
    http.post('/api/v1/tickets/:id/assign', async ({ request }) => { bodies.push(`assign ${JSON.stringify(await request.json())}`); return HttpResponse.json(ticketFixture) }),
    http.post('/api/v1/tickets/:id/transfer', async ({ request }) => { bodies.push(`transfer ${JSON.stringify(await request.json())}`); return HttpResponse.json(ticketFixture) }),
  )
  server.use(http.get('/api/v1/me', () => HttpResponse.json(adminProfileFixture)))
  renderAuthed(<TicketInfo ticket={{ ...ticketFixture, due_at: '2026-09-26T10:00:00Z' }} />)
  await userEvent.click(await screen.findByRole('button', { name: `Subject: ${ticketFixture.subject}` }))
  const subject = screen.getByLabelText('Subject editor')
  await userEvent.clear(subject)
  await userEvent.type(subject, 'New subject')
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))

  await userEvent.click(screen.getByRole('button', { name: `Help Topic: ${ticketFixture.topic!.name}` }))
  await userEvent.selectOptions(await screen.findByLabelText('Help Topic editor'), '')
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))

  await userEvent.click(screen.getByRole('button', { name: /^Assigned To:/ }))
  await userEvent.selectOptions(await screen.findByLabelText('Assigned To editor'), '3')
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))

  await userEvent.click(screen.getByRole('button', { name: /^Department:/ }))
  await userEvent.selectOptions(await screen.findByLabelText('Department editor'), '2')
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))

  await userEvent.click(screen.getByRole('button', { name: `User: ${ticketFixture.requester_name}` }))
  await userEvent.type(screen.getByLabelText('User editor'), 'sy')
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))

  await userEvent.click(screen.getByRole('button', { name: /^Due Date:/ }))
  // The suite runs in America/Los_Angeles (UTC-7 in September).
  expect(screen.getByLabelText('Due Date editor')).toHaveValue('2026-09-26T03:00')
  await userEvent.clear(screen.getByLabelText('Due Date editor'))
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(bodies).toEqual([
    'patch {"subject":"New subject"}',
    'patch {"topic_id":null}',
    'assign {"staff_id":3}',
    'transfer {"dept_id":2}',
    'patch {"requester_name":"Patsy"}',
    'patch {"due_at":null}',
  ]))
})

it('resets a cancelled draft when the editor reopens', async () => {
  renderAuthed(<TicketInfo ticket={ticketFixture} />)
  await userEvent.click(await screen.findByRole('button', { name: `Subject: ${ticketFixture.subject}` }))
  await userEvent.type(screen.getByLabelText('Subject editor'), ' discarded')
  await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  await userEvent.click(screen.getByRole('button', { name: `Subject: ${ticketFixture.subject}` }))
  expect(screen.getByLabelText('Subject editor')).toHaveValue(ticketFixture.subject)
})

it('offers a non-admin only the departments they can see', async () => {
  renderAuthed(<TicketInfo ticket={ticketFixture} />)
  await userEvent.click(await screen.findByRole('button', { name: /^Department:/ }))
  await waitFor(() => expect(within(screen.getByLabelText('Department editor')).getAllByRole('option').map((o) => o.textContent)).toEqual(['Support']))
})

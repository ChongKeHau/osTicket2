import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { server } from '../test/setup'
import { referenceFixtures, ticketFixture } from '../test/fixtures'
import { renderWithProviders } from '../test/render'
import { staffName } from '../lib/format'
import { BannerProvider } from '../ui/BannerContext'
import { TicketActions } from './TicketActions'

const calls: string[] = []
beforeEach(() => {
  calls.length = 0
  server.use(
    http.post('/api/v1/tickets/:id/assign', async ({ request }) => { calls.push(`assign ${JSON.stringify(await request.json())}`); return HttpResponse.json(ticketFixture) }),
    http.post('/api/v1/tickets/:id/transfer', async ({ request }) => { calls.push(`transfer ${JSON.stringify(await request.json())}`); return HttpResponse.json(ticketFixture) }),
    http.post('/api/v1/tickets/:id/status', async ({ request }) => { calls.push(`status ${JSON.stringify(await request.json())}`); return HttpResponse.json(ticketFixture) }),
  )
})

function mount(onCompose = vi.fn()) {
  renderWithProviders(<BannerProvider><TicketActions ticket={ticketFixture} onCompose={onCompose} /></BannerProvider>)
  return onCompose
}

it('offers Post Reply / Post Note and the menus', async () => {
  const onCompose = mount()
  await userEvent.click(screen.getByRole('button', { name: 'Post Reply' }))
  expect(onCompose).toHaveBeenCalledWith('reply')
  await userEvent.click(screen.getByRole('button', { name: 'Post Note' }))
  expect(onCompose).toHaveBeenCalledWith('note')
})

it('assigns through the Assign menu and flashes a notice', async () => {
  mount()
  await userEvent.click(screen.getByRole('button', { name: /Assign/ }))
  const agent = referenceFixtures.staff[0]!
  await userEvent.click(await screen.findByRole('menuitem', { name: staffName(agent) }))
  expect(await screen.findByRole('status')).toHaveTextContent(/assigned/i)
  expect(calls).toEqual([`assign {"staff_id":${agent.id}}`])
})

it('offers only agents who can see the ticket department', async () => {
  mount()
  await userEvent.click(screen.getByRole('button', { name: /Assign/ }))
  await screen.findByRole('menuitem', { name: 'Ann Agent' })
  expect(screen.getAllByRole('menuitem').map((li) => li.textContent)).toEqual(['Ann Agent', 'Root Admin', 'Unassign'])
})

it('changes status and transfers', async () => {
  mount()
  await userEvent.click(screen.getByRole('button', { name: new RegExp(ticketFixture.status.name) }))
  expect(await screen.findByRole('menuitem', { name: ticketFixture.status.name })).toHaveAttribute('aria-disabled', 'true')
  const other = referenceFixtures.statuses.find((st) => st.id !== ticketFixture.status.id)!
  await userEvent.click(screen.getByRole('menuitem', { name: other.name }))
  await waitFor(() => expect(calls).toHaveLength(1))
  await userEvent.click(screen.getByRole('button', { name: /Transfer/ }))
  const dept = referenceFixtures.departments.find((d) => d.id !== ticketFixture.department.id)!
  await userEvent.click(await screen.findByRole('menuitem', { name: dept.name }))
  await waitFor(() => expect(calls).toEqual([`status {"status_id":${other.id}}`, `transfer {"dept_id":${dept.id}}`]))
})

it('flashes the error when an action fails', async () => {
  server.use(http.post('/api/v1/tickets/:id/transfer', () => HttpResponse.json({ error: { code: 'forbidden', message: 'cannot transfer' } }, { status: 403 })))
  mount()
  await userEvent.click(screen.getByRole('button', { name: /Transfer/ }))
  await userEvent.click(await screen.findByRole('menuitem', { name: 'Billing' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('cannot transfer')
})

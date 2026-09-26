import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { delay, http, HttpResponse } from 'msw'
import { server } from '../test/setup'
import { entryFixtures, referenceFixtures, ticketFixture } from '../test/fixtures'
import { renderWithProviders } from '../test/render'
import { BannerProvider } from '../ui/BannerContext'
import { Composer } from './Composer'

function mount(tab: 'reply' | 'note' = 'reply') {
  const onTab = vi.fn()
  const r = renderWithProviders(<BannerProvider><Composer ticketId={ticketFixture.id} tab={tab} onTab={onTab} requesterEmail={ticketFixture.requester_email} /></BannerProvider>)
  return { onTab, client: r.client }
}

it('posts a reply then the status change, in that order', async () => {
  const calls: string[] = []
  server.use(
    http.post('/api/v1/tickets/:id/reply', async ({ request }) => { calls.push(`reply ${JSON.stringify(await request.json())}`); return HttpResponse.json({ ...entryFixtures[1], id: 9 }, { status: 201 }) }),
    http.post('/api/v1/tickets/:id/status', async ({ request }) => { calls.push(`status ${JSON.stringify(await request.json())}`); return HttpResponse.json(ticketFixture) }),
  )
  const { client } = mount()
  const spy = vi.spyOn(client, 'invalidateQueries')
  expect(screen.getByLabelText('To')).toHaveValue(ticketFixture.requester_email)
  await userEvent.type(screen.getByLabelText('Response'), 'Thanks, fixed.')
  const closed = referenceFixtures.statuses.find((st) => st.state === 'closed')!
  await userEvent.selectOptions(await screen.findByLabelText('Set status to'), String(closed.id))
  await userEvent.click(screen.getByRole('button', { name: 'Post Reply' }))
  expect(await screen.findByRole('status')).toHaveTextContent(/Reply posted/)
  expect(calls).toEqual(['reply {"body":"Thanks, fixed.","format":"text","file_ids":[]}', `status {"status_id":${closed.id}}`])
  expect(screen.getByLabelText('Response')).toHaveValue('')
  await waitFor(() => expect(spy).toHaveBeenCalledWith({ queryKey: ['thread', ticketFixture.id] }))
})

it('reports a posted reply whose status change failed, without re-posting', async () => {
  let replies = 0
  server.use(
    http.post('/api/v1/tickets/:id/reply', () => { replies++; return HttpResponse.json({ ...entryFixtures[1], id: 9 }, { status: 201 }) }),
    http.post('/api/v1/tickets/:id/status', () => HttpResponse.json({ error: { code: 'conflict', message: 'locked' } }, { status: 409 })),
  )
  mount()
  await userEvent.type(screen.getByLabelText('Response'), 'x')
  const closed = referenceFixtures.statuses.find((st) => st.state === 'closed')!
  await userEvent.selectOptions(await screen.findByLabelText('Set status to'), String(closed.id))
  await userEvent.click(screen.getByRole('button', { name: 'Post Reply' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(/reply was posted.*status was not changed/i)
  expect(replies).toBe(1)
  expect(screen.getByLabelText('Response')).toHaveValue('')
})

it('switches to the note tab and posts a note', async () => {
  let body: unknown
  server.use(http.post('/api/v1/tickets/:id/notes', async ({ request }) => { body = await request.json(); return HttpResponse.json({ ...entryFixtures[1], id: 10, type: 'note' }, { status: 201 }) }))
  const { onTab } = mount('note')
  expect(screen.getByRole('tab', { name: 'Internal Note' })).toHaveAttribute('aria-selected', 'true')
  await userEvent.type(screen.getByLabelText('Title'), 'Checked logs')
  await userEvent.type(screen.getByLabelText('Note'), 'internal')
  await userEvent.click(screen.getByRole('button', { name: 'Post Note' }))
  expect(await screen.findByRole('status')).toHaveTextContent(/Note posted/)
  expect(body).toEqual({ title: 'Checked logs', body: 'internal', format: 'text', file_ids: [] })
  expect(screen.getByLabelText('Note')).toHaveValue('')
  await userEvent.click(screen.getByRole('tab', { name: 'Reply' }))
  expect(onTab).toHaveBeenCalledWith('reply')
})

it('shows a validation error from the API on the field and keeps the draft', async () => {
  server.use(http.post('/api/v1/tickets/:id/reply', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { body: 'required' } } }, { status: 400 })))
  mount()
  await userEvent.type(screen.getByLabelText('Response'), 'x')
  await userEvent.click(screen.getByRole('button', { name: 'Post Reply' }))
  expect(await screen.findByText('required')).toBeInTheDocument()
  expect(screen.getByLabelText('Response')).toHaveValue('x')
})

it('flashes a non-field error and keeps the draft', async () => {
  server.use(http.post('/api/v1/tickets/:id/reply', () => HttpResponse.json({ error: { code: 'conflict', message: 'Ticket is closed' } }, { status: 409 })))
  mount()
  await userEvent.type(screen.getByLabelText('Response'), 'x')
  await userEvent.click(screen.getByRole('button', { name: 'Post Reply' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Ticket is closed')
  expect(screen.getByLabelText('Response')).toHaveValue('x')
})

it('sends uploaded file ids and disables submit while uploading', async () => {
  let replyBody: unknown
  server.use(
    http.post('/api/v1/files', async () => { await delay(150); return HttpResponse.json({ id: 42, name: 'a.txt', mime: 'text/plain', size: 3 }, { status: 201 }) }),
    http.post('/api/v1/tickets/:id/reply', async ({ request }) => { replyBody = await request.json(); return HttpResponse.json({ id: 9 }, { status: 201 }) }),
  )
  mount()
  await userEvent.type(screen.getByLabelText('Response'), 'see attached')
  await userEvent.upload(screen.getByLabelText(/attach files/i), new File(['abc'], 'a.txt', { type: 'text/plain' }))
  expect(screen.getByText(/a\.txt/)).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Post Reply' })).toBeDisabled()
  await waitFor(() => expect(screen.getByRole('button', { name: 'Post Reply' })).toBeEnabled())
  await userEvent.click(screen.getByRole('button', { name: 'Post Reply' }))
  await waitFor(() => expect(replyBody).toMatchObject({ file_ids: [42] }))
  expect(screen.queryByText(/a\.txt/)).not.toBeInTheDocument()
})

it('removing a file mid-upload keeps it out of file_ids', async () => {
  let replyBody: unknown
  server.use(
    http.post('/api/v1/files', async () => { await delay(300); return HttpResponse.json({ id: 42, name: 'a.txt', mime: 'text/plain', size: 3 }, { status: 201 }) }),
    http.post('/api/v1/tickets/:id/reply', async ({ request }) => { replyBody = await request.json(); return HttpResponse.json({ id: 9 }, { status: 201 }) }),
  )
  mount()
  await userEvent.type(screen.getByLabelText('Response'), 'see attached')
  await userEvent.upload(screen.getByLabelText(/attach files/i), new File(['abc'], 'a.txt', { type: 'text/plain' }))
  await userEvent.click(screen.getByRole('button', { name: /remove a\.txt/i }))
  await new Promise((r) => setTimeout(r, 400))
  expect(screen.queryByText(/a\.txt/)).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: 'Post Reply' }))
  await waitFor(() => expect(replyBody).toMatchObject({ file_ids: [] }))
})

it('failed upload shows the error and does not block posting', async () => {
  server.use(http.post('/api/v1/files', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { file: 'file type application/x-msdownload is not allowed' } } }, { status: 400 })))
  mount()
  await userEvent.type(screen.getByLabelText('Response'), 'body')
  await userEvent.upload(screen.getByLabelText(/attach files/i), new File(['x'], 'evil.exe', { type: 'application/x-msdownload' }))
  expect(await screen.findByText(/not allowed/i)).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Post Reply' })).toBeEnabled()
})

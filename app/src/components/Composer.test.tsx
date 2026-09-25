import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse, delay } from 'msw'
import { REFRESH_KEY, tokens } from '../api/client'
import { sessionFixture } from '../test/fixtures'
import { renderWithProviders } from '../test/render'
import { server } from '../test/setup'
import { Composer } from './Composer'

beforeEach(() => { localStorage.setItem(REFRESH_KEY, 'refresh-1'); tokens.setSession(sessionFixture) })

test('reply posts text body with optional status and invalidates', async () => {
  let body: unknown
  server.use(http.post('/api/v1/tickets/7/reply', async ({ request }) => { body = await request.json(); return HttpResponse.json({ id: 9 }, { status: 201 }) }))
  const { client } = renderWithProviders(<Composer ticketId={7} />)
  const spy = vi.spyOn(client, 'invalidateQueries')
  await userEvent.type(await screen.findByLabelText(/^reply$/i), 'Thanks, fixed.')
  await userEvent.selectOptions(screen.getByLabelText(/set status/i), '3')
  await userEvent.click(screen.getByRole('button', { name: /send reply/i }))
  await waitFor(() => expect(body).toEqual({ body: 'Thanks, fixed.', format: 'text', status_id: 3, file_ids: [] }))
  await waitFor(() => expect(spy).toHaveBeenCalledWith({ queryKey: ['thread', 7] }))
  expect(screen.getByLabelText(/^reply$/i)).toHaveValue('')
})

test('note tab posts to /notes with a title', async () => {
  let body: unknown
  server.use(http.post('/api/v1/tickets/7/notes', async ({ request }) => { body = await request.json(); return HttpResponse.json({ id: 10 }, { status: 201 }) }))
  renderWithProviders(<Composer ticketId={7} />)
  await userEvent.click(screen.getByRole('tab', { name: /internal note/i }))
  await userEvent.type(screen.getByLabelText(/title/i), 'Checked logs')
  await userEvent.type(screen.getByLabelText(/^note$/i), 'Nothing unusual')
  await userEvent.click(screen.getByRole('button', { name: /add note/i }))
  await waitFor(() => expect(body).toEqual({ title: 'Checked logs', body: 'Nothing unusual', format: 'text', file_ids: [] }))
})

test('validation error from the API shows on the field', async () => {
  server.use(http.post('/api/v1/tickets/7/reply', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { body: 'required' } } }, { status: 400 })))
  renderWithProviders(<Composer ticketId={7} />)
  await userEvent.type(screen.getByLabelText(/^reply$/i), 'x')
  await userEvent.click(screen.getByRole('button', { name: /send reply/i }))
  expect(await screen.findByText('required')).toBeInTheDocument()
})

test('upload adds a pending file and submit sends its id; submit is disabled while uploading', async () => {
  let replyBody: unknown
  server.use(
    http.post('/api/v1/files', async () => { await delay(150); return HttpResponse.json({ id: 42, name: 'a.txt', mime: 'text/plain', size: 3 }, { status: 201 }) }),
    http.post('/api/v1/tickets/7/reply', async ({ request }) => { replyBody = await request.json(); return HttpResponse.json({ id: 9 }, { status: 201 }) }),
  )
  renderWithProviders(<Composer ticketId={7} />)
  await userEvent.type(screen.getByLabelText(/^reply$/i), 'see attached')
  await userEvent.upload(screen.getByLabelText(/attach files/i), new File(['abc'], 'a.txt', { type: 'text/plain' }))
  expect(screen.getByText(/a\.txt/)).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /send reply/i })).toBeDisabled()
  await waitFor(() => expect(screen.getByRole('button', { name: /send reply/i })).toBeEnabled())
  await userEvent.click(screen.getByRole('button', { name: /remove a\.txt/i }))
  expect(screen.queryByText(/a\.txt/)).not.toBeInTheDocument()
  await userEvent.upload(screen.getByLabelText(/attach files/i), new File(['abc'], 'a.txt', { type: 'text/plain' }))
  await waitFor(() => expect(screen.getByRole('button', { name: /send reply/i })).toBeEnabled())
  await userEvent.click(screen.getByRole('button', { name: /send reply/i }))
  await waitFor(() => expect(replyBody).toMatchObject({ file_ids: [42] }))
})

test('failed upload shows the error and does not block other files', async () => {
  server.use(http.post('/api/v1/files', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { file: 'file type application/x-msdownload is not allowed' } } }, { status: 400 })))
  renderWithProviders(<Composer ticketId={7} />)
  await userEvent.type(screen.getByLabelText(/^reply$/i), 'body')
  await userEvent.upload(screen.getByLabelText(/attach files/i), new File(['x'], 'evil.exe', { type: 'application/x-msdownload' }))
  expect(await screen.findByText(/not allowed/i)).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /send reply/i })).toBeEnabled()
})

test('removing a file mid-upload keeps it out of file_ids', async () => {
  let replyBody: unknown
  server.use(
    http.post('/api/v1/files', async () => { await delay(300); return HttpResponse.json({ id: 42, name: 'a.txt', mime: 'text/plain', size: 3 }, { status: 201 }) }),
    http.post('/api/v1/tickets/7/reply', async ({ request }) => { replyBody = await request.json(); return HttpResponse.json({ id: 9 }, { status: 201 }) }),
  )
  renderWithProviders(<Composer ticketId={7} />)
  await userEvent.type(screen.getByLabelText(/^reply$/i), 'see attached')
  await userEvent.upload(screen.getByLabelText(/attach files/i), new File(['abc'], 'a.txt', { type: 'text/plain' }))
  await userEvent.click(screen.getByRole('button', { name: /remove a\.txt/i }))
  expect(screen.queryByText(/a\.txt/)).not.toBeInTheDocument()
  await new Promise((r) => setTimeout(r, 400))
  expect(screen.queryByText(/a\.txt/)).not.toBeInTheDocument()
  await waitFor(() => expect(screen.getByRole('button', { name: /send reply/i })).toBeEnabled())
  await userEvent.click(screen.getByRole('button', { name: /send reply/i }))
  await waitFor(() => expect(replyBody).toMatchObject({ file_ids: [] }))
})

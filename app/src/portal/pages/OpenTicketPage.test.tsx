import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { Route, Routes } from 'react-router-dom'
import { PORTAL_REFRESH_KEY } from '../../api/portalClient'
import { portalFixtures } from '../../test/portal'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/setup'
import { PortalAuthProvider } from '../PortalAuthContext'
import { PortalShell } from '../PortalShell'
import { TicketOpenedPage } from './CheckEmailPage'
import { OpenTicketPage } from './OpenTicketPage'

function mount(route = '/portal/open') {
  return renderWithProviders(
    <PortalAuthProvider>
      <Routes>
        <Route path="/portal" element={<PortalShell />}>
          <Route path="open" element={<OpenTicketPage />} />
          <Route path="opened/:number" element={<TicketOpenedPage />} />
          <Route path="login" element={<h2>Login page</h2>} />
          <Route path="tickets" element={<h2>My Tickets</h2>} />
          <Route path="tickets/:id" element={<h2>Ticket page</h2>} />
        </Route>
      </Routes>
    </PortalAuthProvider>,
    { route },
  )
}

it('anonymous: shows Name, Email, Help Topic, Department, Subject, Message and Attachments', async () => {
  mount()
  expect(await screen.findByLabelText(/^name$/i)).toBeInTheDocument()
  const email = screen.getByLabelText(/^email/i)
  expect(email).toBeRequired()
  expect(email).not.toHaveAttribute('readonly')
  expect(screen.getByLabelText(/help topic/i)).toBeInTheDocument()
  expect(screen.getByLabelText(/^department$/i)).toBeInTheDocument()
  expect(screen.getByLabelText(/^subject/i)).toBeRequired()
  expect(screen.getByLabelText(/^message/i)).toBeRequired()
  expect(screen.getByLabelText(/attach files/i)).toBeInTheDocument()
})

it('anonymous: submits the ticket and shows the emailed-link confirmation', async () => {
  let body: unknown
  server.use(http.post('/api/v1/portal/tickets', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ id: 8, number: '000008' }, { status: 201 })
  }))
  mount()
  await userEvent.type(await screen.findByLabelText(/^name$/i), 'Jamie Customer')
  await userEvent.type(screen.getByLabelText(/^email/i), 'jamie@example.test')
  await userEvent.type(screen.getByLabelText(/^subject/i), 'Printer on fire')
  await userEvent.type(screen.getByLabelText(/^message/i), 'Please help')
  await userEvent.click(screen.getByRole('button', { name: /open ticket/i }))
  await waitFor(() => expect(body).toEqual({
    name: 'Jamie Customer', email: 'jamie@example.test', subject: 'Printer on fire', message: 'Please help',
    format: 'text', topic_id: 0, dept_id: 0, file_ids: [], file_tokens: [],
  }))
  expect(await screen.findByRole('heading', { name: 'Ticket #000008 opened' })).toBeInTheDocument()
  expect(screen.getByText(/we emailed you a link to follow this ticket/i)).toBeInTheDocument()
})

it('anonymous: sends each uploaded file id with its access token', async () => {
  let body: { file_ids?: number[]; file_tokens?: string[] } = {}
  server.use(http.post('/api/v1/portal/tickets', async ({ request }) => {
    body = (await request.json()) as typeof body
    return HttpResponse.json({ id: 8, number: '000008' }, { status: 201 })
  }))
  mount()
  await userEvent.type(await screen.findByLabelText(/^name$/i), 'Jamie Customer')
  await userEvent.type(screen.getByLabelText(/^email/i), 'jamie@example.test')
  await userEvent.type(screen.getByLabelText(/^subject/i), 'Printer on fire')
  await userEvent.type(screen.getByLabelText(/^message/i), 'Please help')
  await userEvent.upload(screen.getByLabelText(/attach files/i), new File(['abc'], 'a.txt', { type: 'text/plain' }))
  await waitFor(() => expect(screen.queryByText('uploading…')).not.toBeInTheDocument())
  await userEvent.click(screen.getByRole('button', { name: /open ticket/i }))
  await waitFor(() => expect(body.file_ids).toEqual([42]))
  expect(body.file_tokens).toEqual(['tok-42'])
})

it('signed in: email is prefilled and read-only; success view has a View ticket link and no email note', async () => {
  localStorage.setItem(PORTAL_REFRESH_KEY, 'prefresh-1')
  let body: unknown
  server.use(http.post('/api/v1/portal/tickets', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ id: 8, number: '000008' }, { status: 201 })
  }))
  mount()
  const email = await screen.findByLabelText(/^email/i)
  expect(email).toHaveValue(portalFixtures.profile.email)
  expect(email).toHaveAttribute('readonly')
  await userEvent.type(screen.getByLabelText(/^subject/i), 'Printer on fire')
  await userEvent.type(screen.getByLabelText(/^message/i), 'Please help')
  await userEvent.click(screen.getByRole('button', { name: /open ticket/i }))
  await waitFor(() => expect(body).toMatchObject({ email: portalFixtures.profile.email, subject: 'Printer on fire' }))
  expect(await screen.findByRole('heading', { name: 'Ticket #000008 opened' })).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'View ticket' })).toHaveAttribute('href', '/portal/tickets/8')
  expect(screen.queryByText(/we emailed you a link/i)).not.toBeInTheDocument()
})

it('a 400 with fields.subject shows the error under Subject', async () => {
  server.use(http.post('/api/v1/portal/tickets', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { subject: 'must be under 255 characters' } } }, { status: 400 })))
  mount()
  await userEvent.type(await screen.findByLabelText(/^name$/i), 'Jamie Customer')
  await userEvent.type(screen.getByLabelText(/^email/i), 'jamie@example.test')
  await userEvent.type(screen.getByLabelText(/^subject/i), 'Printer on fire')
  await userEvent.type(screen.getByLabelText(/^message/i), 'Please help')
  await userEvent.click(screen.getByRole('button', { name: /open ticket/i }))
  const subject = await screen.findByLabelText(/^subject/i)
  expect(within(subject.closest('tr') as HTMLElement).getByText('must be under 255 characters')).toBeInTheDocument()
})

it('a 429 shows the too-many-attempts banner with minutes derived from retry_after', async () => {
  server.use(http.post('/api/v1/portal/tickets', () =>
    HttpResponse.json({ error: { code: 'rate_limited', message: 'too many attempts', fields: { retry_after: '600' } } }, { status: 429 })))
  mount()
  await userEvent.type(await screen.findByLabelText(/^name$/i), 'Jamie Customer')
  await userEvent.type(screen.getByLabelText(/^email/i), 'jamie@example.test')
  await userEvent.type(screen.getByLabelText(/^subject/i), 'Printer on fire')
  await userEvent.type(screen.getByLabelText(/^message/i), 'Please help')
  await userEvent.click(screen.getByRole('button', { name: /open ticket/i }))
  expect(await screen.findByText('Too many attempts, try again in 10 minutes')).toBeInTheDocument()
})

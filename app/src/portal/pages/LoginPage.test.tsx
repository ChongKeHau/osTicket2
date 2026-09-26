import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { portal } from '../../api/portalClient'
import { makeQueryClient } from '../../test/render'
import { server } from '../../test/setup'
import { PortalAuthProvider } from '../PortalAuthContext'
import { PortalShell } from '../PortalShell'
import { LoginPage } from './LoginPage'

beforeEach(() => portal.tokens.clear())

function mount(state?: unknown) {
  return render(
    <QueryClientProvider client={makeQueryClient()}>
      <MemoryRouter initialEntries={[{ pathname: '/portal/login', state }]}>
        <PortalAuthProvider>
          <Routes>
            <Route path="/portal" element={<PortalShell />}>
              <Route path="login" element={<LoginPage />} />
              <Route path="tickets" element={<h2>My Tickets page</h2>} />
              <Route path="tickets/:id" element={<h2>Ticket page</h2>} />
            </Route>
          </Routes>
        </PortalAuthProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

const signIn = () => screen.getByRole('region', { name: 'Sign in' })
const guest = () => screen.getByRole('region', { name: 'Check a ticket as a guest' })

async function fillSignIn(email: string, password: string) {
  await screen.findByRole('heading', { name: 'Sign in' })
  await userEvent.type(within(signIn()).getByLabelText(/^email/i), email)
  await userEvent.type(within(signIn()).getByLabelText(/^password/i), password)
  await userEvent.click(within(signIn()).getByRole('button', { name: 'Sign In' }))
}

it('shows the sign-in and guest columns with account links', async () => {
  mount()
  expect(await screen.findByRole('heading', { name: 'Sign in' })).toBeInTheDocument()
  expect(screen.getByRole('heading', { name: 'Check a ticket as a guest' })).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Create an account' })).toHaveAttribute('href', '/portal/register')
  expect(screen.getByRole('link', { name: 'Forgot password' })).toHaveAttribute('href', '/portal/reset')
})

it('password sign-in posts { email, password } and goes to the ticket list', async () => {
  let body: unknown
  server.use(http.post('/api/v1/portal/auth/login', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({
      access_token: 'paccess-1', refresh_token: 'prefresh-1', expires_in: 900, ticket_id: null,
      user: { id: 3, email: 'pat@example.test', name: 'Pat Customer', verified: true, has_password: true },
    })
  }))
  mount()
  await fillSignIn('pat@example.test', 'secret123')
  expect(await screen.findByRole('heading', { name: 'My Tickets page' })).toBeInTheDocument()
  expect(body).toEqual({ email: 'pat@example.test', password: 'secret123' })
})

it('password sign-in returns to the page in state.from', async () => {
  mount({ from: '/portal/tickets/7' })
  await fillSignIn('pat@example.test', 'secret123')
  expect(await screen.findByRole('heading', { name: 'Ticket page' })).toBeInTheDocument()
})

it('a wrong password shows the generic error', async () => {
  mount()
  await fillSignIn('pat@example.test', 'wrong-pass')
  expect(await screen.findByText('Invalid email or password')).toBeInTheDocument()
  expect(screen.getByRole('heading', { name: 'Sign in' })).toBeInTheDocument()
})

it('"Email me a sign-in link" posts { email } and shows the check-email page', async () => {
  let body: unknown
  server.use(http.post('/api/v1/portal/auth/link', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({}, { status: 202 })
  }))
  mount()
  await screen.findByRole('heading', { name: 'Sign in' })
  await userEvent.type(within(signIn()).getByLabelText(/^email/i), 'pat@example.test')
  await userEvent.click(within(signIn()).getByRole('button', { name: 'Email me a sign-in link' }))
  expect(await screen.findByText('If that address has an account, we sent a sign-in link')).toBeInTheDocument()
  expect(body).toEqual({ email: 'pat@example.test' })
})

it('"Email me a sign-in link" with no email focuses the field with an error and sends nothing', async () => {
  let called = false
  server.use(http.post('/api/v1/portal/auth/link', () => { called = true; return HttpResponse.json({}, { status: 202 }) }))
  mount()
  await screen.findByRole('heading', { name: 'Sign in' })
  await userEvent.click(within(signIn()).getByRole('button', { name: 'Email me a sign-in link' }))
  expect(within(signIn()).getByLabelText(/^email/i)).toHaveFocus()
  expect(within(signIn()).getByText('Enter your email address')).toBeInTheDocument()
  expect(called).toBe(false)
})

it('the guest form posts { email, number } to /access and shows the check-email page', async () => {
  let body: unknown
  server.use(http.post('/api/v1/portal/auth/access', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({}, { status: 202 })
  }))
  mount()
  await screen.findByRole('heading', { name: 'Check a ticket as a guest' })
  await userEvent.type(within(guest()).getByLabelText(/^email/i), 'jamie@example.test')
  await userEvent.type(within(guest()).getByLabelText(/ticket number/i), '000007')
  await userEvent.click(within(guest()).getByRole('button', { name: 'Email me an access link' }))
  expect(await screen.findByText('If the ticket and email match, we sent an access link')).toBeInTheDocument()
  await waitFor(() => expect(body).toEqual({ email: 'jamie@example.test', number: '000007' }))
})

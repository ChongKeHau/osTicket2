import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { Route, Routes } from 'react-router-dom'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/setup'
import { RegisterPage } from './RegisterPage'

function mount() {
  return renderWithProviders(
    <Routes>
      <Route path="/portal/register" element={<RegisterPage />} />
      <Route path="/portal/login" element={<h2>Login page</h2>} />
    </Routes>,
    { route: '/portal/register' },
  )
}

async function fill(name: string, email: string) {
  await userEvent.type(screen.getByLabelText(/^name/i), name)
  await userEvent.type(screen.getByLabelText(/^email/i), email)
  await userEvent.click(screen.getByRole('button', { name: 'Create Account' }))
}

it('asks for Name and Email only', () => {
  mount()
  expect(screen.getByLabelText(/^name/i)).toBeRequired()
  expect(screen.getByLabelText(/^email/i)).toBeRequired()
  expect(screen.queryByLabelText(/password/i)).not.toBeInTheDocument()
})

it('posts { email, name } and shows the confirm-your-email page', async () => {
  let body: unknown
  server.use(http.post('/api/v1/portal/auth/register', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({}, { status: 201 })
  }))
  mount()
  await fill('Jamie Customer', 'jamie@example.test')
  expect(await screen.findByText('Check your email to confirm your account')).toBeInTheDocument()
  await waitFor(() => expect(body).toEqual({ email: 'jamie@example.test', name: 'Jamie Customer' }))
})

it('shows the same confirmation for an address that already has an account', async () => {
  // The API answers 201 {} either way, so the page cannot (and must not) tell.
  mount()
  await fill('Pat Customer', 'pat@example.test')
  expect(await screen.findByText('Check your email to confirm your account')).toBeInTheDocument()
  expect(screen.queryByText(/already exists/i)).not.toBeInTheDocument()
})

it('a 400 with fields.email shows the error under Email', async () => {
  server.use(http.post('/api/v1/portal/auth/register', () =>
    HttpResponse.json({ error: { code: 'validation_failed', message: 'request validation failed', fields: { email: 'must be a valid email' } } }, { status: 400 })))
  mount()
  await fill('Jamie', 'jamie@example.test')
  expect(await screen.findByText('must be a valid email')).toBeInTheDocument()
})

it('a 429 shows the rate-limit message', async () => {
  server.use(http.post('/api/v1/portal/auth/register', () =>
    HttpResponse.json({ error: { code: 'rate_limited', message: 'too many attempts', fields: { retry_after: '600' } } }, { status: 429 })))
  mount()
  await fill('Jamie', 'jamie@example.test')
  expect(await screen.findByText('Too many attempts, try again in 10 minutes')).toBeInTheDocument()
})

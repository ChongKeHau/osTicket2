import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { Route, Routes } from 'react-router-dom'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/setup'
import { ResetPage } from './ResetPage'

it('posts the email to /auth/reset and shows the check-email page', async () => {
  let body: unknown
  server.use(http.post('/api/v1/portal/auth/reset', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({}, { status: 202 })
  }))
  renderWithProviders(
    <Routes><Route path="/portal/reset" element={<ResetPage />} /></Routes>,
    { route: '/portal/reset' },
  )
  await userEvent.type(screen.getByLabelText(/^email/i), 'pat@example.test')
  await userEvent.click(screen.getByRole('button', { name: 'Email me a reset link' }))
  expect(await screen.findByRole('heading', { name: 'Check your email' })).toBeInTheDocument()
  expect(screen.getByText('If that address has an account, we sent a password reset link')).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Back to sign in' })).toHaveAttribute('href', '/portal/login')
  await waitFor(() => expect(body).toEqual({ email: 'pat@example.test' }))
})

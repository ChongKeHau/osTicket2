import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { CheckEmailPage, TicketOpenedPage } from './CheckEmailPage'

it('renders the heading, body and an optional link from props', () => {
  render(
    <MemoryRouter>
      <CheckEmailPage heading="Check your email" body="We sent you a confirmation link." linkTo="/portal/login" linkLabel="Back to sign in" />
    </MemoryRouter>,
  )
  expect(screen.getByRole('heading', { name: 'Check your email' })).toBeInTheDocument()
  expect(screen.getByText('We sent you a confirmation link.')).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Back to sign in' })).toHaveAttribute('href', '/portal/login')
})

it('omits the link when none is given', () => {
  render(<MemoryRouter><CheckEmailPage heading="Check your email" body="Body text" /></MemoryRouter>)
  expect(screen.getByRole('heading', { name: 'Check your email' })).toBeInTheDocument()
  expect(screen.queryByRole('link')).not.toBeInTheDocument()
})

function mountOpened(state?: unknown) {
  return render(
    <MemoryRouter initialEntries={[{ pathname: '/portal/opened/000008', state }]}>
      <Routes><Route path="/portal/opened/:number" element={<TicketOpenedPage />} /></Routes>
    </MemoryRouter>,
  )
}

it('anonymous: shows the emailed-link note and no ticket link', () => {
  mountOpened({ id: 8, anonymous: true })
  expect(screen.getByRole('heading', { name: 'Ticket #000008 opened' })).toBeInTheDocument()
  expect(screen.getByText(/we emailed you a link to follow this ticket/i)).toBeInTheDocument()
  expect(screen.queryByRole('link')).not.toBeInTheDocument()
})

it('signed in: shows a View ticket link and no email note', () => {
  mountOpened({ id: 8, anonymous: false })
  expect(screen.getByRole('heading', { name: 'Ticket #000008 opened' })).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'View ticket' })).toHaveAttribute('href', '/portal/tickets/8')
  expect(screen.queryByText(/we emailed you a link/i)).not.toBeInTheDocument()
})

it('with no state at all (defensive default), still renders the heading without crashing', () => {
  mountOpened(undefined)
  expect(screen.getByRole('heading', { name: 'Ticket #000008 opened' })).toBeInTheDocument()
})

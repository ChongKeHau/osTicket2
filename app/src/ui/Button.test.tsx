import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { Button, LinkButton } from './Button'

it('applies the variant class and forwards props', () => {
  render(<Button variant="add" disabled>Add New</Button>)
  const b = screen.getByRole('button', { name: 'Add New' })
  expect(b).toBeDisabled()
  expect(b.className).toContain('add')
})

it('renders a link button', () => {
  render(<MemoryRouter><LinkButton to="/tickets/new" variant="primary">New Ticket</LinkButton></MemoryRouter>)
  expect(screen.getByRole('link', { name: 'New Ticket' })).toHaveAttribute('href', '/tickets/new')
})

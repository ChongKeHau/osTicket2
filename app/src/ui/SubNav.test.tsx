import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { SubNav } from './SubNav'

it('marks the item whose target equals path+search', () => {
  render(
    <MemoryRouter initialEntries={['/tickets?state=open']}>
      <SubNav items={[{ label: 'Open', to: '/tickets?state=open' }, { label: 'Closed', to: '/tickets?state=closed' }]} right={<a href="/tickets/new">New Ticket</a>} />
    </MemoryRouter>,
  )
  expect(screen.getByRole('link', { name: 'Open' })).toHaveAttribute('aria-current', 'page')
  expect(screen.getByRole('link', { name: 'Closed' })).not.toHaveAttribute('aria-current')
  expect(screen.getByRole('link', { name: 'New Ticket' })).toBeInTheDocument()
})

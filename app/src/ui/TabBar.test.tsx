import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { TabBar } from './TabBar'

it('marks the tab matching the path prefix as current', () => {
  render(<MemoryRouter initialEntries={['/tickets/7']}><TabBar tabs={[{ label: 'Dashboard', to: '/dashboard' }, { label: 'Tickets', to: '/tickets' }]} /></MemoryRouter>)
  expect(screen.getByRole('link', { name: 'Tickets' })).toHaveAttribute('aria-current', 'page')
  expect(screen.getByRole('link', { name: 'Dashboard' })).not.toHaveAttribute('aria-current')
})

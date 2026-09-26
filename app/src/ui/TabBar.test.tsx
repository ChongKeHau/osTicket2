import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { TabBar } from './TabBar'

it('marks the tab matching the path prefix as current', () => {
  render(<MemoryRouter initialEntries={['/tickets/7']}><TabBar tabs={[{ label: 'Dashboard', to: '/dashboard' }, { label: 'Tickets', to: '/tickets' }]} /></MemoryRouter>)
  expect(screen.getByRole('link', { name: 'Tickets' })).toHaveAttribute('aria-current', 'page')
  expect(screen.getByRole('link', { name: 'Dashboard' })).not.toHaveAttribute('aria-current')
})

it('matches an `end` tab only on its exact path', () => {
  const tabs = [{ label: 'Home', to: '/portal', end: true }, { label: 'Open', to: '/portal/open' }]
  const { unmount } = render(<MemoryRouter initialEntries={['/portal/open']}><TabBar tabs={tabs} /></MemoryRouter>)
  expect(screen.getByRole('link', { name: 'Home' })).not.toHaveAttribute('aria-current')
  expect(screen.getByRole('link', { name: 'Open' })).toHaveAttribute('aria-current', 'page')
  unmount()
  render(<MemoryRouter initialEntries={['/portal']}><TabBar tabs={tabs} /></MemoryRouter>)
  expect(screen.getByRole('link', { name: 'Home' })).toHaveAttribute('aria-current', 'page')
})

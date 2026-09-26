import { render, screen } from '@testing-library/react'
import { StickyBar } from './StickyBar'

it('renders title, count and actions', () => {
  render(<StickyBar title="Open Tickets" count={12} actions={<button>Act</button>} />)
  expect(screen.getByRole('heading', { level: 2 })).toHaveTextContent('Open Tickets (12)')
  expect(screen.getByRole('button', { name: 'Act' })).toBeInTheDocument()
})

import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Menu } from './Menu'

const items = [
  { label: 'Edit', onSelect: vi.fn() },
  { label: 'Delete', onSelect: vi.fn(), danger: true },
  { label: 'Disabled', onSelect: vi.fn(), disabled: true },
]

it('opens on click, selects with keyboard, closes on Escape and restores focus', async () => {
  render(<Menu label="More" items={items} />)
  const trigger = screen.getByRole('button', { name: 'More' })
  expect(screen.queryByRole('menu')).not.toBeInTheDocument()
  await userEvent.click(trigger)
  expect(screen.getByRole('menu')).toBeInTheDocument()
  await userEvent.keyboard('{ArrowDown}{ArrowDown}{Enter}')
  expect(items[1]!.onSelect).toHaveBeenCalled()
  expect(screen.queryByRole('menu')).not.toBeInTheDocument()
  await userEvent.click(trigger)
  await userEvent.keyboard('{Escape}')
  expect(screen.queryByRole('menu')).not.toBeInTheDocument()
  expect(trigger).toHaveFocus()
})

it('closes on outside click and skips disabled items', async () => {
  render(<><Menu label="More" items={items} /><button>outside</button></>)
  await userEvent.click(screen.getByRole('button', { name: 'More' }))
  expect(screen.getByRole('menuitem', { name: 'Disabled' })).toHaveAttribute('aria-disabled', 'true')
  await userEvent.click(screen.getByRole('button', { name: 'outside' }))
  expect(screen.queryByRole('menu')).not.toBeInTheDocument()
})

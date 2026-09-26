import { render, screen } from '@testing-library/react'
import { Badge } from './Badge'

it('uses the given colour as background', () => {
  render(<Badge color="#ff0000">High</Badge>)
  expect(screen.getByText('High')).toHaveStyle({ backgroundColor: '#ff0000' })
})

it.each(['#FFFFFF', '#DDFFDD', '#fff'])('uses dark text on the light colour %s', (color) => {
  render(<Badge color={color}>Light</Badge>)
  expect(screen.getByText('Light').style.color).toBe('var(--text)')
})

it.each(['#FF0000', '#1a5f9a', '#000'])('uses white text on the dark colour %s', (color) => {
  render(<Badge color={color}>Dark</Badge>)
  expect(screen.getByText('Dark').style.color).toBe('var(--on-accent)')
})

it('lets fg override the derived text colour', () => {
  render(<Badge color="#FFFFFF" fg="red">Forced</Badge>)
  expect(screen.getByText('Forced').style.color).toBe('red')
})

it('leaves a badge without a colour unstyled', () => {
  render(<Badge>Plain</Badge>)
  expect(screen.getByText('Plain').getAttribute('style')).toBeNull()
})

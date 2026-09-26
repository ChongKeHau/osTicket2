import { render, screen } from '@testing-library/react'
import { Badge } from './Badge'

it('uses the given colour as background', () => {
  render(<Badge color="#ff0000">High</Badge>)
  expect(screen.getByText('High')).toHaveStyle({ backgroundColor: '#ff0000' })
})

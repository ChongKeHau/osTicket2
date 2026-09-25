import { screen } from '@testing-library/react'
import { renderWithProviders } from './test/render'
import App from './App'

test('renders the app title', () => {
  renderWithProviders(<App />)
  expect(screen.getByText(/ticket desk/i)).toBeInTheDocument()
})

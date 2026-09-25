import { screen } from '@testing-library/react'
import App from './App'
import { renderWithProviders } from './test/render'

test('unknown route renders not found', async () => {
  localStorage.setItem('ticket.refresh_token', 'refresh-1')
  renderWithProviders(<App />, { route: '/nope' })
  expect(await screen.findByText(/page not found/i)).toBeInTheDocument()
})

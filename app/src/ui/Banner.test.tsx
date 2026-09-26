import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Banner, errorMessage } from './Banner'
import { BannerProvider, useBanner } from './BannerContext'

it('uses alert role for error and warning, status otherwise', () => {
  render(<><Banner level="error">bad</Banner><Banner level="notice">ok</Banner></>)
  expect(screen.getByRole('alert')).toHaveTextContent('bad')
  expect(screen.getByRole('status')).toHaveTextContent('ok')
})

it('dismisses', async () => {
  const onDismiss = vi.fn()
  render(<Banner level="info" onDismiss={onDismiss}>hi</Banner>)
  await userEvent.click(screen.getByRole('button', { name: 'Dismiss' }))
  expect(onDismiss).toHaveBeenCalled()
})

it('formats errors', () => {
  expect(errorMessage(new ApiError(404, 'not_found', 'Ticket not found'))).toBe('Ticket not found')
  expect(errorMessage(new Error('boom'))).toBe('boom')
  expect(errorMessage('x')).toBe('Something went wrong')
})

function Flasher() {
  const { flash } = useBanner()
  return <button onClick={() => flash('notice', 'Saved')}>go</button>
}

it('flashes through the provider', async () => {
  render(<MemoryRouter><BannerProvider><Flasher /></BannerProvider></MemoryRouter>)
  await userEvent.click(screen.getByText('go'))
  expect(screen.getByRole('status')).toHaveTextContent('Saved')
})

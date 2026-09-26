import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { MemoryRouter, useNavigate } from 'react-router-dom'
import { afterEach } from 'vitest'
import { BannerProvider, useBanner } from './BannerContext'

function Flasher() {
  const { flash } = useBanner()
  const navigate = useNavigate()
  return (
    <>
      <button onClick={() => flash('notice', 'Note posted')}>go</button>
      <button onClick={() => navigate('/elsewhere')}>nav</button>
    </>
  )
}

const wrap = (ui: ReactNode) => render(<MemoryRouter initialEntries={['/start']}>{ui}</MemoryRouter>)

afterEach(() => {
  delete (Element.prototype as { scrollIntoView?: unknown }).scrollIntoView
  vi.useRealTimers()
})

it('scrolls a new flash into view', async () => {
  const scroll = vi.fn()
  Element.prototype.scrollIntoView = scroll
  wrap(<BannerProvider><Flasher /></BannerProvider>)
  expect(scroll).not.toHaveBeenCalled()
  await userEvent.click(screen.getByText('go'))
  expect(screen.getByRole('status')).toHaveTextContent('Note posted')
  expect(scroll).toHaveBeenCalledWith({ block: 'nearest' })
})

it('flashes when scrollIntoView is unavailable (jsdom)', async () => {
  wrap(<BannerProvider><Flasher /></BannerProvider>)
  await userEvent.click(screen.getByText('go'))
  expect(screen.getByRole('status')).toHaveTextContent('Note posted')
})

it('keeps a flash across a navigation that follows it immediately', () => {
  vi.useFakeTimers()
  wrap(<BannerProvider><Flasher /></BannerProvider>)
  fireEvent.click(screen.getByText('go'))
  act(() => { vi.advanceTimersByTime(200) })
  fireEvent.click(screen.getByText('nav'))
  expect(screen.getByRole('status')).toHaveTextContent('Note posted')
})

it('clears a flash on a later navigation', () => {
  vi.useFakeTimers()
  wrap(<BannerProvider><Flasher /></BannerProvider>)
  fireEvent.click(screen.getByText('go'))
  expect(screen.getByRole('status')).toHaveTextContent('Note posted')
  act(() => { vi.advanceTimersByTime(1500) })
  fireEvent.click(screen.getByText('nav'))
  expect(screen.queryByRole('status')).not.toBeInTheDocument()
})

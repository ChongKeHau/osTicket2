import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach } from 'vitest'
import { BannerProvider, useBanner } from './BannerContext'

function Flasher() {
  const { flash } = useBanner()
  return <button onClick={() => flash('notice', 'Note posted')}>go</button>
}

afterEach(() => {
  delete (Element.prototype as { scrollIntoView?: unknown }).scrollIntoView
})

it('scrolls a new flash into view', async () => {
  const scroll = vi.fn()
  Element.prototype.scrollIntoView = scroll
  render(<BannerProvider><Flasher /></BannerProvider>)
  expect(scroll).not.toHaveBeenCalled()
  await userEvent.click(screen.getByText('go'))
  expect(screen.getByRole('status')).toHaveTextContent('Note posted')
  expect(scroll).toHaveBeenCalledWith({ block: 'nearest' })
})

it('flashes when scrollIntoView is unavailable (jsdom)', async () => {
  render(<BannerProvider><Flasher /></BannerProvider>)
  await userEvent.click(screen.getByText('go'))
  expect(screen.getByRole('status')).toHaveTextContent('Note posted')
})

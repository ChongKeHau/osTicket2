import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Pagination } from './Pagination'

it('shows the range and pages', async () => {
  const onPage = vi.fn()
  render(<Pagination page={2} pageSize={25} total={132} onPage={onPage} />)
  expect(screen.getByText('Showing 26–50 of 132')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Page 2' })).toHaveAttribute('aria-current', 'page')
  await userEvent.click(screen.getByRole('button', { name: 'Next' }))
  expect(onPage).toHaveBeenCalledWith(3)
  await userEvent.click(screen.getByRole('button', { name: 'Page 6' }))
  expect(onPage).toHaveBeenCalledWith(6)
})

it('hides itself for a single page and clamps the range', () => {
  const { container } = render(<Pagination page={1} pageSize={25} total={0} onPage={() => {}} />)
  expect(container).toBeEmptyDOMElement()
})

it('disables prev on the first page', () => {
  render(<Pagination page={1} pageSize={10} total={30} onPage={() => {}} />)
  expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()
})

import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Tabs } from './Tabs'

it('renders a tablist and switches', async () => {
  const onChange = vi.fn()
  render(<Tabs tabs={[{ id: 'reply', label: 'Reply' }, { id: 'note', label: 'Internal Note' }]} active="reply" onChange={onChange}><p>panel</p></Tabs>)
  expect(screen.getByRole('tab', { name: 'Reply' })).toHaveAttribute('aria-selected', 'true')
  await userEvent.click(screen.getByRole('tab', { name: 'Internal Note' }))
  expect(onChange).toHaveBeenCalledWith('note')
  expect(screen.getByRole('tabpanel')).toHaveTextContent('panel')
})

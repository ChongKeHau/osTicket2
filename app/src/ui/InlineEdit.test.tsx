import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { InlineEdit } from './InlineEdit'

it('opens the editor, saves, and cancels on Escape with focus restored', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined)
  render(<InlineEdit label="Status" value="Open" editor={() => <select aria-label="Status editor"><option>Open</option></select>} onSave={onSave} />)
  const trigger = screen.getByRole('button', { name: 'Status: Open' })
  await userEvent.click(trigger)
  expect(screen.getByLabelText('Status editor')).toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  expect(onSave).toHaveBeenCalled()
  expect(screen.queryByLabelText('Status editor')).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: 'Status: Open' }))
  await userEvent.keyboard('{Escape}')
  expect(screen.queryByLabelText('Status editor')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Status: Open' })).toHaveFocus()
})

it('stays open and shows the error when save rejects', async () => {
  const onSave = vi.fn().mockRejectedValue(new Error('nope'))
  render(<InlineEdit label="Priority" value="Low" editor={() => <input aria-label="p" />} onSave={onSave} />)
  await userEvent.click(screen.getByRole('button', { name: 'Priority: Low' }))
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('nope')
  expect(screen.getByLabelText('p')).toBeInTheDocument()
})

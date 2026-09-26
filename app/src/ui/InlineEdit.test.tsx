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

it('moves focus into the editor on open', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined)
  render(<InlineEdit label="Status" value="Open" editor={() => <select aria-label="Status editor"><option>Open</option></select>} onSave={onSave} />)
  await userEvent.click(screen.getByRole('button', { name: 'Status: Open' }))
  expect(screen.getByLabelText('Status editor')).toHaveFocus()
})

it('Escape in one open editor closes only that editor', async () => {
  const onSaveA = vi.fn().mockResolvedValue(undefined)
  const onSaveB = vi.fn().mockResolvedValue(undefined)
  render(
    <>
      <InlineEdit label="A" value="x" editor={() => <input aria-label="A editor" />} onSave={onSaveA} />
      <InlineEdit label="B" value="y" editor={() => <input aria-label="B editor" />} onSave={onSaveB} />
    </>,
  )
  await userEvent.click(screen.getByRole('button', { name: 'A: x' }))
  await userEvent.click(screen.getByRole('button', { name: 'B: y' }))
  expect(screen.getByLabelText('A editor')).toBeInTheDocument()
  expect(screen.getByLabelText('B editor')).toBeInTheDocument()
  await userEvent.keyboard('{Escape}')
  expect(screen.queryByLabelText('B editor')).not.toBeInTheDocument()
  expect(screen.getByLabelText('A editor')).toBeInTheDocument()
})

import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfirmDelete } from './ConfirmDelete'

test('delete asks for confirmation, then calls onConfirm and resets', async () => {
  const onConfirm = vi.fn(() => Promise.resolve())
  render(<ConfirmDelete label="Sales" onConfirm={onConfirm} />)
  await userEvent.click(screen.getByRole('button', { name: 'Delete Sales' }))
  expect(onConfirm).not.toHaveBeenCalled()
  await userEvent.click(screen.getByRole('button', { name: /confirm/i }))
  expect(onConfirm).toHaveBeenCalledTimes(1)
  await waitFor(() => expect(screen.getByRole('button', { name: 'Delete Sales' })).toBeInTheDocument())
})

test('cancel returns to the initial state without calling onConfirm', async () => {
  const onConfirm = vi.fn(() => Promise.resolve())
  render(<ConfirmDelete label="Sales" onConfirm={onConfirm} />)
  await userEvent.click(screen.getByRole('button', { name: 'Delete Sales' }))
  await userEvent.click(screen.getByRole('button', { name: /cancel/i }))
  expect(onConfirm).not.toHaveBeenCalled()
  expect(screen.getByRole('button', { name: 'Delete Sales' })).toBeInTheDocument()
})

test('disabled renders the Delete button disabled', () => {
  render(<ConfirmDelete label="Sales" onConfirm={() => Promise.resolve()} disabled />)
  expect(screen.getByRole('button', { name: 'Delete Sales' })).toBeDisabled()
})

test('disabled also disables Confirm and Cancel once confirming', async () => {
  const onConfirm = vi.fn(() => Promise.resolve())
  const { rerender } = render(<ConfirmDelete label="Sales" onConfirm={onConfirm} />)
  await userEvent.click(screen.getByRole('button', { name: 'Delete Sales' }))
  rerender(<ConfirmDelete label="Sales" onConfirm={onConfirm} disabled />)
  expect(screen.getByRole('button', { name: 'Confirm delete Sales' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Cancel delete Sales' })).toBeDisabled()
})

test('Confirm and Cancel name the record for screen readers', async () => {
  render(<ConfirmDelete label="Sales" onConfirm={() => Promise.resolve()} />)
  await userEvent.click(screen.getByRole('button', { name: 'Delete Sales' }))
  expect(screen.getByRole('button', { name: 'Confirm delete Sales' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Cancel delete Sales' })).toBeInTheDocument()
})

test('a rejected onConfirm resets to the initial state and disables buttons while pending', async () => {
  let reject!: (e: Error) => void
  const onConfirm = vi.fn(() => new Promise<void>((_, rej) => { reject = rej }))
  render(<ConfirmDelete label="Sales" onConfirm={onConfirm} />)
  await userEvent.click(screen.getByRole('button', { name: 'Delete Sales' }))
  await userEvent.click(screen.getByRole('button', { name: /confirm/i }))
  expect(screen.getByRole('button', { name: /confirm/i })).toBeDisabled()
  expect(screen.getByRole('button', { name: /cancel/i })).toBeDisabled()
  reject(new Error('conflict'))
  await waitFor(() => expect(screen.getByRole('button', { name: 'Delete Sales' })).toBeInTheDocument())
})

test('startConfirming opens on Confirm/Cancel and Cancel calls onCancel', async () => {
  const onCancel = vi.fn()
  const onConfirm = vi.fn(() => Promise.resolve())
  render(<ConfirmDelete label="Sales" onConfirm={onConfirm} startConfirming onCancel={onCancel} />)
  expect(screen.queryByRole('button', { name: 'Delete Sales' })).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: 'Cancel delete Sales' }))
  expect(onCancel).toHaveBeenCalledTimes(1)
  expect(onConfirm).not.toHaveBeenCalled()
})

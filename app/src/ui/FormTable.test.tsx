import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { FormActions } from './FormActions'
import { FormTable } from './FormTable'

it('renders sections, required markers and errors', () => {
  render(
    <FormTable sections={[{ title: 'Settings', rows: [
      { id: 'name', label: 'Name', required: true, error: 'required', control: <input id="name" /> },
      { id: 'pub', label: 'Public', help: 'Visible to users', control: <input id="pub" type="checkbox" /> },
    ] }]} />,
  )
  expect(screen.getByText('Settings')).toBeInTheDocument()
  expect(screen.getByLabelText(/Name/)).toHaveAttribute('aria-invalid', 'true')
  expect(screen.getByText('required')).toBeInTheDocument()
  expect(screen.getByText('Visible to users')).toBeInTheDocument()
  expect(screen.getByText('*')).toBeInTheDocument()
})

it('describes a help-only control without marking it invalid', () => {
  render(
    <FormTable sections={[{ title: 'Settings', rows: [
      { id: 'pub', label: 'Public', help: 'Visible to users', control: <input id="pub" type="checkbox" /> },
    ] }]} />,
  )
  const control = screen.getByLabelText('Public')
  expect(control).toHaveAttribute('aria-describedby', 'pub-help')
  expect(control).not.toHaveAttribute('aria-invalid')
})

it('renders form actions', () => {
  render(<MemoryRouter><FormActions saving cancelTo="/admin/departments" onReset={() => {}} /></MemoryRouter>)
  expect(screen.getByRole('button', { name: 'Save Changes' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Reset' })).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Cancel' })).toHaveAttribute('href', '/admin/departments')
})

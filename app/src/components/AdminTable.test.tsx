import { render, screen, within } from '@testing-library/react'
import { AdminTable } from './AdminTable'

const rows = [{ id: 1, name: 'Support', is_public: true }, { id: 2, name: 'Sales', is_public: false }]
const columns = [
  { header: 'Name', cell: (r: typeof rows[number]) => r.name },
  { header: 'Public', cell: (r: typeof rows[number]) => (r.is_public ? 'Yes' : 'No') },
]

test('renders headers, one row per item, and the actions column', () => {
  render(<AdminTable columns={columns} rows={rows} actions={(r) => <button type="button">Edit {r.name}</button>} />)
  expect(screen.getAllByRole('columnheader').map((h) => h.textContent)).toEqual(['Name', 'Public', ''])
  const body = screen.getAllByRole('row').slice(1) as HTMLElement[]
  expect(body).toHaveLength(2)
  expect(within(body[1]!).getAllByRole('cell').map((c) => c.textContent)).toEqual(['Sales', 'No', 'Edit Sales'])
})

test('renders the empty message when there are no rows', () => {
  render(<AdminTable columns={columns} rows={[]} empty="No departments yet." />)
  expect(screen.getByText('No departments yet.')).toBeInTheDocument()
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
})

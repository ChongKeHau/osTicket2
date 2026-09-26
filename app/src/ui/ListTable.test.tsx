import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { ListTable, type Column } from './ListTable'

type R = { id: number; name: string; n: number }
const cols: Column<R>[] = [
  { key: 'name', label: 'Name', sortKey: 'name', render: (r) => r.name },
  { key: 'n', label: 'Count', align: 'right', render: (r) => r.n },
]
const rows = [{ id: 1, name: 'a', n: 1 }, { id: 2, name: 'b', n: 2 }]

it('renders rows as links and toggles sort', async () => {
  const onSort = vi.fn()
  render(<MemoryRouter><ListTable columns={cols} rows={rows} rowKey={(r) => r.id} sort="-name" onSort={onSort} rowHref={(r) => `/r/${r.id}`} /></MemoryRouter>)
  expect(screen.getAllByRole('row')).toHaveLength(3)
  expect(screen.getByRole('link', { name: 'a' })).toHaveAttribute('href', '/r/1')
  const header = screen.getByRole('columnheader', { name: /Name/ })
  expect(header).toHaveAttribute('aria-sort', 'descending')
  await userEvent.click(screen.getByRole('button', { name: /Name/ }))
  expect(onSort).toHaveBeenCalledWith('name')
  expect(screen.getByRole('columnheader', { name: 'Count' })).not.toHaveAttribute('aria-sort')
})

it('renders the empty state and footer', () => {
  render(<MemoryRouter><ListTable columns={cols} rows={[]} rowKey={(r) => r.id} footer={<span>foot</span>} /></MemoryRouter>)
  expect(screen.getByText('No records found')).toBeInTheDocument()
  expect(screen.getByText('foot')).toBeInTheDocument()
})

it('applies cellClass to the td', () => {
  const colsWithCellClass: Column<R>[] = [
    { key: 'name', label: 'Name', render: (r) => r.name, cellClass: (r) => (r.n > 1 ? 'big' : undefined) },
  ]
  render(<MemoryRouter><ListTable columns={colsWithCellClass} rows={rows} rowKey={(r) => r.id} /></MemoryRouter>)
  const cells = screen.getAllByRole('cell')
  expect(cells[0]!.className).not.toContain('big')
  expect(cells[1]!.className).toContain('big')
})

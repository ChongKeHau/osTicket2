import type { ReactNode } from 'react'

export interface Column<T> { header: string; cell: (row: T) => ReactNode }

export function AdminTable<T extends { id: number }>({ columns, rows, actions, empty = 'Nothing here yet.' }: {
  columns: Column<T>[]; rows: T[]; actions?: (row: T) => ReactNode; empty?: string
}) {
  if (rows.length === 0) return <p className="muted">{empty}</p>
  return (
    <table>
      <thead>
        <tr>
          {columns.map((c) => <th key={c.header}>{c.header}</th>)}
          {actions && <th aria-label="Actions" />}
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => (
          <tr key={row.id}>
            {columns.map((c) => <td key={c.header}>{c.cell(row)}</td>)}
            {actions && <td style={{ textAlign: 'right' }}>{actions(row)}</td>}
          </tr>
        ))}
      </tbody>
    </table>
  )
}

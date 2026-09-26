import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import s from './ListTable.module.css'

export interface Column<T> { key: string; label: string; sortKey?: string; width?: string; align?: 'left' | 'right'; render: (row: T) => ReactNode; cellClass?: (row: T) => string | undefined }

interface Props<T> {
  columns: Column<T>[]; rows: T[]; rowKey: (row: T) => string | number
  sort?: string; onSort?: (sort: string) => void; rowHref?: (row: T) => string; footer?: ReactNode; empty?: string
}

export function toggleSort(current: string | undefined, key: string): string {
  if (current === key) return `-${key}`
  return key
}

export function ListTable<T>({ columns, rows, rowKey, sort, onSort, rowHref, footer, empty = 'No records found' }: Props<T>) {
  const active = sort?.replace(/^-/, '')
  const desc = sort?.startsWith('-')
  return (
    <div className={s.wrap}>
      <table className={s.table}>
        <thead>
          <tr>
            {columns.map((c) => {
              const isActive = c.sortKey !== undefined && c.sortKey === active
              const ariaSort = isActive ? (desc ? 'descending' : 'ascending') : undefined
              return (
                <th key={c.key} style={{ width: c.width }} className={c.align === 'right' ? s.right : undefined} aria-sort={ariaSort} scope="col">
                  {c.sortKey && onSort ? (
                    <button type="button" className={s.sortBtn} onClick={() => onSort(toggleSort(sort, c.sortKey!))}>
                      {c.label} <span aria-hidden="true" className={isActive ? s.arrowOn : s.arrowOff}>{isActive ? (desc ? '▼' : '▲') : '▲▼'}</span>
                    </button>
                  ) : c.label}
                </th>
              )
            })}
          </tr>
        </thead>
        <tbody>
          {rows.length === 0 && <tr><td colSpan={columns.length} className={s.empty}>{empty}</td></tr>}
          {rows.map((r) => (
            <tr key={rowKey(r)} className={rowHref ? s.linkRow : undefined}>
              {columns.map((c, i) => (
                <td key={c.key} className={[c.align === 'right' ? s.right : undefined, c.cellClass?.(r)].filter(Boolean).join(' ') || undefined}>
                  {rowHref && i === 0 ? <Link to={rowHref(r)} className={s.rowLink}>{c.render(r)}</Link> : c.render(r)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
        {footer && <tfoot><tr><td colSpan={columns.length} className={s.footer}>{footer}</td></tr></tfoot>}
      </table>
    </div>
  )
}

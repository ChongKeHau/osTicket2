import type { ReactNode } from 'react'
import s from './StickyBar.module.css'

export function StickyBar({ title, count, actions }: { title: ReactNode; count?: number; actions?: ReactNode }) {
  return (
    <div className={s.bar}>
      <h2 className={s.title}>{title}{count !== undefined && <> <span className={s.count}>({count})</span></>}</h2>
      {actions && <div className={s.actions}>{actions}</div>}
    </div>
  )
}

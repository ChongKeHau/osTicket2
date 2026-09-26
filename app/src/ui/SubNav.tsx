import type { ReactNode } from 'react'
import { Link, useLocation } from 'react-router-dom'
import s from './SubNav.module.css'

export interface SubNavItem { label: string; to: string; active?: boolean }

export function SubNav({ items, right }: { items: SubNavItem[]; right?: ReactNode }) {
  const loc = useLocation()
  const here = loc.pathname + loc.search
  return (
    <nav className={s.bar} aria-label="Secondary">
      <div className={s.inner}>
        <ul className={s.list}>
          {items.map((it) => {
            const active = it.active ?? here === it.to
            return (
              <li key={it.to}>
                <Link to={it.to} className={active ? `${s.item} ${s.active}` : s.item} aria-current={active ? 'page' : undefined}>{it.label}</Link>
              </li>
            )
          })}
        </ul>
        {right && <div className={s.right}>{right}</div>}
      </div>
    </nav>
  )
}

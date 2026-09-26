import { NavLink } from 'react-router-dom'
import s from './TabBar.module.css'

export function TabBar({ tabs }: { tabs: { label: string; to: string }[] }) {
  return (
    <nav className={s.bar} aria-label="Primary">
      <ul className={s.list}>
        {tabs.map((t) => (
          <li key={t.to}>
            <NavLink to={t.to} className={({ isActive }) => (isActive ? `${s.tab} ${s.active}` : s.tab)}>{t.label}</NavLink>
          </li>
        ))}
      </ul>
    </nav>
  )
}

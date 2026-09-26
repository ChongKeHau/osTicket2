import { NavLink } from 'react-router-dom'
import s from './TabBar.module.css'

/** Tabs match their path as a prefix; `end` restricts a tab to an exact match (e.g. a section's home). */
export function TabBar({ tabs }: { tabs: { label: string; to: string; end?: boolean }[] }) {
  return (
    <nav className={s.bar} aria-label="Primary">
      <ul className={s.list}>
        {tabs.map((t) => (
          <li key={t.to}>
            <NavLink to={t.to} end={t.end} className={({ isActive }) => (isActive ? `${s.tab} ${s.active}` : s.tab)}>{t.label}</NavLink>
          </li>
        ))}
      </ul>
    </nav>
  )
}

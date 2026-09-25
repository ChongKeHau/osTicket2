import { NavLink, Outlet } from 'react-router-dom'
import styles from './admin.module.css'

export function AdminLayout() {
  return (
    <div>
      <nav className={styles.subnav} aria-label="Admin sections">
        <NavLink to="/admin/departments">Departments</NavLink>
        <NavLink to="/admin/topics">Topics</NavLink>
        <NavLink to="/admin/staff">Staff</NavLink>
      </nav>
      <Outlet />
    </div>
  )
}

import { Link, NavLink, Outlet } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'
import { staffName } from '../lib/format'
import styles from './Layout.module.css'

export function Layout() {
  const { staff, logout } = useAuth()
  return (
    <div className={styles.shell}>
      <header className={styles.header}>
        <Link to="/tickets" className={styles.brand}>Ticket Desk</Link>
        <nav className={styles.nav}>
          <NavLink to="/tickets" end>Tickets</NavLink>
          <NavLink to="/tickets/new">New ticket</NavLink>
        </nav>
        <div className={styles.user}>
          {staff && <span>{staffName(staff)}</span>}
          <button type="button" onClick={() => void logout()}>Sign out</button>
        </div>
      </header>
      <main className={styles.main}>
        <Outlet />
      </main>
    </div>
  )
}

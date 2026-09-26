import { createContext, useContext, useLayoutEffect, useMemo, useState, type ReactNode } from 'react'
import { Link, Outlet } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'
import { ADMIN_TABS, AGENT_TABS } from '../nav'
import { staffName } from '../lib/format'
import { BannerProvider } from './BannerContext'
import { SubNav, type SubNavItem } from './SubNav'
import { TabBar } from './TabBar'
import s from './AppShell.module.css'

interface SubNavState { items: SubNavItem[]; right?: ReactNode }
const SubNavCtx = createContext<((st: SubNavState | null) => void) | null>(null)

/** Pages call this to populate the sub-nav strip for as long as they are mounted.
 *  The effect keys on the serialised items, so an inline array is fine; `right`
 *  is compared by identity, so pass a stable element or memoise it (`useMemo`). */
export function useSubNav(items: SubNavItem[], right?: ReactNode) {
  const set = useContext(SubNavCtx)
  const key = JSON.stringify(items)
  useLayoutEffect(() => {
    set?.({ items, right })
    return () => set?.(null)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [set, key, right])
}

export function AppShell({ panel, children }: { panel: 'agent' | 'admin'; children?: ReactNode }) {
  const { staff, isAdmin, logout } = useAuth()
  const [sub, setSub] = useState<SubNavState | null>(null)
  const tabs = panel === 'admin' ? ADMIN_TABS : AGENT_TABS
  const setter = useMemo(() => setSub, [])
  return (
    <div className={s.page}>
      <header className={s.header}>
        <div className={s.headerInner}>
          <Link to="/tickets" className={s.brand}>Ticket Desk</Link>
          <div className={s.user}>
            {staff && <span className={s.welcome}>Welcome, <strong>{staffName(staff)}</strong></span>}
            {isAdmin && (panel === 'admin'
              ? <Link to="/tickets" className={s.headerLink}>Agent Panel</Link>
              : <Link to="/admin/departments" className={s.headerLink}>Admin Panel</Link>)}
            <button type="button" className={s.headerBtn} onClick={() => void logout()}>Log Out</button>
          </div>
        </div>
      </header>
      <TabBar tabs={tabs} />
      {sub && <SubNav items={sub.items} right={sub.right} />}
      <main className={s.content}>
        <SubNavCtx.Provider value={setter}>
          <BannerProvider>
            {children ?? <Outlet />}
          </BannerProvider>
        </SubNavCtx.Provider>
      </main>
      <footer className={s.footer}>Ticket Desk</footer>
    </div>
  )
}

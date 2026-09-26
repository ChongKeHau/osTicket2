import { createContext, useContext, useLayoutEffect, useState, type ReactNode } from 'react'
import { Link, Outlet } from 'react-router-dom'
import { BannerProvider } from '../ui/BannerContext'
import { SubNav, type SubNavItem } from '../ui/SubNav'
import { TabBar } from '../ui/TabBar'
import { usePortalAuth } from './PortalAuthContext'
import { PORTAL_MY_TICKETS_TAB, PORTAL_TABS } from './portalNav'
import s from './PortalShell.module.css'

const PortalSubNavCtx = createContext<((items: SubNavItem[] | null) => void) | null>(null)

/** Pages call this to populate the portal sub-nav strip for as long as they are mounted.
 *  The effect keys on the serialised items, so an inline array is fine. */
export function usePortalSubNav(items: SubNavItem[]) {
  const set = useContext(PortalSubNavCtx)
  const key = JSON.stringify(items)
  useLayoutEffect(() => {
    set?.(items)
    return () => set?.(null)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [set, key])
}

/** Customer-portal chrome: header user bar, tabs, optional sub-nav, centred panel, footer. */
export function PortalShell({ children }: { children?: ReactNode }) {
  const { status, user, isGuest, ticketId, logout } = usePortalAuth()
  const [sub, setSub] = useState<SubNavItem[] | null>(null)
  const tabs = status === 'authenticated' && !isGuest
    ? PORTAL_TABS.map((t) => (t.to === '/portal/login' ? PORTAL_MY_TICKETS_TAB : t))
    : PORTAL_TABS
  return (
    <div className={s.page}>
      <header className={s.header}>
        <div className={s.headerInner}>
          <Link to="/portal" className={s.brand}>Ticket Desk</Link>
          <div className={s.user}>
            {status !== 'authenticated' && <>
              <span className={s.muted}>Guest User</span>
              <Link to="/portal/login" className={s.headerLink}>Sign In</Link>
            </>}
            {status === 'authenticated' && isGuest && <>
              <span className={s.muted}>Ticket access</span>
              <Link to={`/portal/tickets/${ticketId}`} className={s.headerLink}>My Ticket</Link>
              <button type="button" className={s.headerBtn} onClick={() => void logout()}>Sign Out</button>
            </>}
            {status === 'authenticated' && !isGuest && user && <>
              <span><strong>{user.name || user.email}</strong></span>
              <Link to="/portal/profile" className={s.headerLink}>Profile</Link>
              <Link to="/portal/tickets" className={s.headerLink}>Tickets</Link>
              <button type="button" className={s.headerBtn} onClick={() => void logout()}>Sign Out</button>
            </>}
          </div>
        </div>
      </header>
      <TabBar tabs={tabs} />
      {sub && <SubNav items={sub} />}
      <main className={s.content}>
        <PortalSubNavCtx.Provider value={setSub}>
          <BannerProvider>{children ?? <Outlet />}</BannerProvider>
        </PortalSubNavCtx.Provider>
      </main>
      <footer className={s.footer}>Ticket Desk · Support Center</footer>
    </div>
  )
}

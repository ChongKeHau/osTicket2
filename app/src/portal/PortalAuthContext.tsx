import { useQueryClient } from '@tanstack/react-query'
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { getMe, login as apiLogin, logout as apiLogout } from '../api/portal'
import { portal } from '../api/portalClient'
import { ApiError } from '../api/sessionStore'
import type { PortalProfile, PortalSession } from '../api/types'

type Status = 'loading' | 'anonymous' | 'authenticated' | 'error'

export interface PortalAuthValue {
  status: Status
  user: PortalProfile | null
  /** The ticket a guest session is scoped to; null for an account session or when signed out. */
  ticketId: number | null
  isGuest: boolean
  notice: string | null
  /** Set when status is 'error': restoring the session failed for a reason other than rejection. */
  error: unknown
  login(email: string, password: string): Promise<void>
  /** Takes over a session obtained elsewhere (an emailed-token exchange). */
  adopt(session: PortalSession): void
  logout(): Promise<void>
  /** Re-run the session restore after an 'error' status. */
  retry(): void
  /** Updates the signed-in user in place (no network, no status transition) so chrome that
   *  reads it (e.g. the shell header) reflects a profile change made elsewhere on the same
   *  session, without waiting for the next full session restore. */
  setUser(profile: PortalProfile): void
}

const PortalAuthContext = createContext<PortalAuthValue | null>(null)
const EXPIRED = 'Your session expired. Please sign in again.'
const UNREACHABLE = 'Could not reach the server to restore your session.'

/** The customer-portal session, stored under its own key; never touches the staff session. */
export function PortalAuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<Status>('loading')
  const [user, setUserState] = useState<PortalProfile | null>(null)
  const [ticketId, setTicketId] = useState<number | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [error, setError] = useState<unknown>(null)
  const [attempt, setAttempt] = useState(0)
  /** Bumped by sign-in, adopt and sign-out so a restore still in flight cannot overwrite them. */
  const epoch = useRef(0)
  const queryClient = useQueryClient()

  useEffect(() => {
    let cancelled = false
    const mine = ++epoch.current
    const stale = () => cancelled || epoch.current !== mine
    portal.tokens.setOnSessionLost(() => {
      if (cancelled) return
      setUserState(null)
      setTicketId(null)
      setStatus('anonymous')
      setNotice(EXPIRED)
      queryClient.clear()
    })
    const fail = (e: unknown) => { setError(e); setStatus('error') }
    ;(async () => {
      if (!portal.tokens.getRefresh()) { if (!stale()) setStatus('anonymous'); return }
      const ok = await portal.refreshSession()
      if (stale()) return
      if (!ok) {
        // Token still stored: the refresh failed transiently (network, 5xx), so offer a retry.
        if (portal.tokens.getRefresh()) fail(new ApiError(0, 'network', UNREACHABLE))
        else { setStatus('anonymous'); setNotice(EXPIRED) }
        return
      }
      try {
        // Guest sessions may read /me too; its ticket_id is the scope the API enforces.
        const { ticket_id: scope, ...profile } = await getMe()
        if (stale()) return
        setUserState(profile)
        setTicketId(scope ?? null)
        setStatus('authenticated')
      } catch (e) {
        if (stale()) return
        // 401: the session is gone. 403: a session that may not read /me — either way a retry
        // cannot succeed, so end it cleanly; on 403 the (just rotated) refresh token is still
        // live on the server, so revoke it rather than only forgetting it.
        if (e instanceof ApiError && (e.status === 401 || e.status === 403)) {
          if (e.status === 403) await apiLogout().catch(() => portal.tokens.clear())
          else portal.tokens.clear()
          if (stale()) return
          setStatus('anonymous')
          if (e.status === 401) setNotice(EXPIRED)
        }
        else fail(e)
      }
    })()
    return () => { cancelled = true; portal.tokens.setOnSessionLost(null) }
  }, [queryClient, attempt])

  const retry = useCallback(() => {
    setError(null)
    setStatus('loading')
    setAttempt((n) => n + 1)
  }, [])

  const settle = useCallback((s: PortalSession) => {
    epoch.current++
    setUserState(s.user)
    setTicketId(s.ticket_id)
    setNotice(null)
    setError(null)
    setStatus('authenticated')
  }, [])

  const login = useCallback(async (email: string, password: string) => {
    const s = await apiLogin(email, password)
    settle(s)
  }, [settle])

  const adopt = useCallback((s: PortalSession) => {
    // clear() first so a restore refresh still in flight cannot store its (older) session over this one.
    portal.tokens.clear()
    portal.tokens.setSession(s)
    queryClient.clear()
    settle(s)
  }, [queryClient, settle])

  /** Updates `user` in place; does not touch status, tickets or the query cache. */
  const setUser = useCallback((profile: PortalProfile) => setUserState(profile), [])

  const logout = useCallback(async () => {
    epoch.current++
    try {
      await apiLogout()
    } catch {
      // The local tokens are cleared regardless; a failed server-side revoke only leaves an orphan.
    }
    setUserState(null)
    setTicketId(null)
    setStatus('anonymous')
    setNotice(null)
    queryClient.clear()
  }, [queryClient])

  const value = useMemo<PortalAuthValue>(() => ({
    status, user, ticketId, notice, error, login, adopt, logout, retry, setUser,
    isGuest: ticketId !== null,
  }), [status, user, ticketId, notice, error, login, adopt, logout, retry, setUser])

  return <PortalAuthContext.Provider value={value}>{children}</PortalAuthContext.Provider>
}

export function usePortalAuth(): PortalAuthValue {
  const v = useContext(PortalAuthContext)
  if (!v) throw new Error('usePortalAuth must be used inside PortalAuthProvider')
  return v
}

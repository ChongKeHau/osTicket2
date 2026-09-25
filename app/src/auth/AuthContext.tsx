import { useQueryClient } from '@tanstack/react-query'
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { login as apiLogin, logout as apiLogout, me } from '../api/auth'
import { ApiError, refreshSession, tokens } from '../api/client'
import type { StaffProfile } from '../api/types'

type Status = 'loading' | 'anonymous' | 'authenticated' | 'error'

export interface AuthValue {
  status: Status
  staff: StaffProfile | null
  isAdmin: boolean
  departmentIds: number[]
  notice: string | null
  /** Set when status is 'error': restoring the session failed for a reason other than rejection. */
  error: unknown
  login(username: string, password: string): Promise<void>
  logout(): Promise<void>
  /** Re-run the session restore after an 'error' status. */
  retry(): void
}

const AuthContext = createContext<AuthValue | null>(null)
const EXPIRED = 'Your session expired. Please sign in again.'
const UNREACHABLE = 'Could not reach the server to restore your session.'

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<Status>('loading')
  const [staff, setStaff] = useState<StaffProfile | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [error, setError] = useState<unknown>(null)
  const [attempt, setAttempt] = useState(0)
  const queryClient = useQueryClient()

  useEffect(() => {
    let cancelled = false
    tokens.setOnSessionLost(() => {
      if (cancelled) return
      setStaff(null)
      setStatus('anonymous')
      setNotice(EXPIRED)
      queryClient.clear()
    })
    const fail = (e: unknown) => { setError(e); setStatus('error') }
    ;(async () => {
      if (!tokens.getRefresh()) { if (!cancelled) setStatus('anonymous'); return }
      const ok = await refreshSession()
      if (cancelled) return
      if (!ok) {
        // Token still stored: the refresh failed transiently (network, 5xx), so offer a retry.
        if (tokens.getRefresh()) fail(new ApiError(0, 'network', UNREACHABLE))
        else { setStatus('anonymous'); setNotice(EXPIRED) }
        return
      }
      try {
        const profile = await me()
        if (cancelled) return
        setStaff(profile)
        setStatus('authenticated')
      } catch (e) {
        if (cancelled) return
        if (e instanceof ApiError && e.status === 401) { tokens.clear(); setStatus('anonymous'); setNotice(EXPIRED) }
        else fail(e)
      }
    })()
    return () => { cancelled = true; tokens.setOnSessionLost(null) }
  }, [queryClient, attempt])

  const retry = useCallback(() => {
    setError(null)
    setStatus('loading')
    setAttempt((n) => n + 1)
  }, [])

  const login = useCallback(async (username: string, password: string) => {
    const s = await apiLogin(username, password)
    setStaff(s.staff)
    setNotice(null)
    setError(null)
    setStatus('authenticated')
  }, [])

  const logout = useCallback(async () => {
    await apiLogout()
    setStaff(null)
    setStatus('anonymous')
    setNotice(null)
    queryClient.clear()
  }, [queryClient])

  const value = useMemo<AuthValue>(() => ({
    status, staff, notice, error, login, logout, retry,
    isAdmin: staff?.is_admin ?? false,
    departmentIds: staff?.department_ids ?? [],
  }), [status, staff, notice, error, login, logout, retry])

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthValue {
  const v = useContext(AuthContext)
  if (!v) throw new Error('useAuth must be used inside AuthProvider')
  return v
}

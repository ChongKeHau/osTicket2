import { useQueryClient } from '@tanstack/react-query'
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { login as apiLogin, logout as apiLogout, me } from '../api/auth'
import { refreshSession, tokens } from '../api/client'
import type { StaffProfile } from '../api/types'

type Status = 'loading' | 'anonymous' | 'authenticated'

export interface AuthValue {
  status: Status
  staff: StaffProfile | null
  isAdmin: boolean
  departmentIds: number[]
  notice: string | null
  login(username: string, password: string): Promise<void>
  logout(): Promise<void>
}

const AuthContext = createContext<AuthValue | null>(null)
const EXPIRED = 'Your session expired. Please sign in again.'

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<Status>('loading')
  const [staff, setStaff] = useState<StaffProfile | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
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
    ;(async () => {
      if (!tokens.getRefresh()) { if (!cancelled) setStatus('anonymous'); return }
      const ok = await refreshSession()
      if (cancelled) return
      if (!ok) { setStatus('anonymous'); setNotice(EXPIRED); return }
      try {
        const profile = await me()
        if (cancelled) return
        setStaff(profile)
        setStatus('authenticated')
      } catch {
        if (!cancelled) { tokens.clear(); setStatus('anonymous'); setNotice(EXPIRED) }
      }
    })()
    return () => { cancelled = true; tokens.setOnSessionLost(null) }
  }, [queryClient])

  const login = useCallback(async (username: string, password: string) => {
    const s = await apiLogin(username, password)
    setStaff(s.staff)
    setNotice(null)
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
    status, staff, notice, login, logout,
    isAdmin: staff?.is_admin ?? false,
    departmentIds: staff?.department_ids ?? [],
  }), [status, staff, notice, login, logout])

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthValue {
  const v = useContext(AuthContext)
  if (!v) throw new Error('useAuth must be used inside AuthProvider')
  return v
}

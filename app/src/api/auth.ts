import { request, tokens } from './client'
import type { Session, StaffProfile } from './types'

export async function login(username: string, password: string): Promise<Session> {
  const s = await request<Session>('POST', '/auth/login', { body: { username, password } })
  tokens.setSession(s)
  return s
}

export async function logout(): Promise<void> {
  const raw = tokens.getRefresh()
  if (raw) {
    try { await request<void>('POST', '/auth/logout', { body: { refresh_token: raw } }) } catch { /* clear locally regardless */ }
  }
  tokens.clear()
}

export function me(): Promise<StaffProfile> {
  return request<StaffProfile>('GET', '/me')
}

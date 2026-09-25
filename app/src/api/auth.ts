import { ApiError, refreshSession, request, tokens, waitForRefresh } from './client'
import type { Session, StaffProfile } from './types'

export async function login(username: string, password: string): Promise<Session> {
  const s = await request<Session>('POST', '/auth/login', { body: { username, password } })
  tokens.setSession(s)
  return s
}

function postLogout(): Promise<void> | null {
  // Read the token at call time: a refresh may just have rotated it.
  const raw = tokens.getRefresh()
  return raw ? request<void>('POST', '/auth/logout', { body: { refresh_token: raw } }) : null
}

/**
 * Revoke the refresh token on the server, then clear it locally regardless of the outcome.
 * /auth/logout needs a valid access token, so renew an expired one first.
 */
export async function logout(): Promise<void> {
  try {
    await waitForRefresh()
    if (tokens.access === null) await refreshSession()
    try {
      await postLogout()
    } catch (e) {
      if (!(e instanceof ApiError && e.status === 401)) throw e
      if (await refreshSession()) await postLogout()
    }
  } catch { /* clear locally regardless */ }
  tokens.clear()
}

export function me(): Promise<StaffProfile> {
  return request<StaffProfile>('GET', '/me')
}

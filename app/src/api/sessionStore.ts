import type { ApiErrorBody } from './types'

export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly fields: Record<string, string>

  constructor(status: number, code: string, message: string, fields: Record<string, string> = {}) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.fields = fields
  }
}

export function safeStorage<T>(fn: () => T, fallback: T): T {
  try { return fn() } catch { return fallback }
}

export type Query = Record<string, string | number | boolean | undefined | null>
export interface FetchInit { body?: unknown; formData?: FormData; query?: Query; headers?: Record<string, string> }

/** The fields every session response carries (staff `Session` and `PortalSession` both satisfy it). */
export interface SessionLike { access_token: string; refresh_token: string }
export interface SessionStoreOptions { storageKey: string; base: string; refreshPath: string }

export async function toError(res: Response): Promise<ApiError> {
  try {
    const data = (await res.json()) as Partial<ApiErrorBody>
    if (data.error && typeof data.error.code === 'string') {
      return new ApiError(res.status, data.error.code, data.error.message ?? res.statusText, data.error.fields ?? {})
    }
  } catch { /* not JSON */ }
  return new ApiError(res.status, 'network', `request failed with status ${res.status}`)
}

/**
 * One independent session: an in-memory access token, a refresh token under `storageKey`, and
 * authenticated fetch helpers against `base`. The staff app and the customer portal each own one,
 * so neither can read, rotate or clear the other's tokens. `S` is the session response shape;
 * the last one set (by sign-in or refresh) is kept in memory as `tokens.session`.
 */
export function createSessionStore<S extends SessionLike = SessionLike>(o: SessionStoreOptions) {
  let accessToken: string | null = null
  let current: S | null = null
  let refreshing: Promise<boolean> | null = null
  let sessionLostHandler: (() => void) | null = null
  /** Bumped by tokens.clear() so an in-flight refresh cannot restore a session that was just cleared. */
  let generation = 0

  const tokens = {
    get access(): string | null { return accessToken },
    /** The last session response stored (sign-in, exchange or refresh); null once cleared. */
    get session(): S | null { return current },
    getRefresh(): string | null { return safeStorage(() => localStorage.getItem(o.storageKey), null) },
    /** A session without a refresh token (e.g. a portal reset session) lives in memory only. */
    setSession(s: S): void {
      accessToken = s.access_token
      current = s
      const refresh = s.refresh_token
      safeStorage(() => (refresh
        ? localStorage.setItem(o.storageKey, refresh)
        : localStorage.removeItem(o.storageKey)), undefined)
    },
    clear(): void {
      generation++
      accessToken = null
      current = null
      safeStorage(() => localStorage.removeItem(o.storageKey), undefined)
    },
    setOnSessionLost(fn: (() => void) | null): void { sessionLostHandler = fn },
  }

  function buildUrl(path: string, query?: Query): string {
    const url = new URL(o.base + path, window.location.origin)
    if (query) {
      for (const [k, v] of Object.entries(query)) {
        if (v === undefined || v === null || v === '') continue
        url.searchParams.set(k, String(v))
      }
    }
    return url.toString()
  }

  async function doRefresh(canRetry: boolean): Promise<boolean> {
    const sent = tokens.getRefresh()
    if (!sent) return false
    const gen = generation
    let res: Response
    let session: S | null = null
    try {
      res = await fetch(buildUrl(o.refreshPath), {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ refresh_token: sent }),
      })
      if (res.ok) session = (await res.json()) as S
    } catch {
      return false // network failure or bad body: transient, keep the refresh token
    }
    // tokens.clear() ran while the request was in flight (e.g. sign out): do not resurrect the session.
    if (gen !== generation) return false
    if (session) { tokens.setSession(session); return true }
    if (res.status !== 401 && res.status !== 403) return false // 5xx etc.: transient, keep the token
    const current = tokens.getRefresh()
    if (current !== null && current !== sent) {
      // Another tab rotated the shared token while ours was in flight; use the new one once.
      return canRetry ? doRefresh(false) : false
    }
    tokens.clear()
    sessionLostHandler?.()
    return false
  }

  /**
   * Exchange the stored refresh token for a new session. Concurrent callers share one request.
   * Resolves false without clearing anything on a transient failure (network error, 5xx); clears the
   * tokens and notifies the session-lost handler only when the server rejects the token (401/403).
   */
  function refreshSession(): Promise<boolean> {
    if (refreshing) return refreshing
    if (!tokens.getRefresh()) return Promise.resolve(false)
    refreshing = (async () => {
      try { return await doRefresh(true) } finally { refreshing = null }
    })()
    return refreshing
  }

  /** Resolves once any in-flight refresh has settled (immediately if none). */
  function waitForRefresh(): Promise<unknown> {
    return refreshing ?? Promise.resolve()
  }

  async function fetchWithAuth(method: string, path: string, init: FetchInit = {}, retry = true): Promise<Response> {
    const headers: Record<string, string> = { Accept: 'application/json', ...init.headers }
    if (accessToken) headers['Authorization'] = `Bearer ${accessToken}`
    let body: BodyInit | undefined
    if (init.formData) body = init.formData
    else if (init.body !== undefined) { headers['Content-Type'] = 'application/json'; body = JSON.stringify(init.body) }
    const res = await fetch(buildUrl(path, init.query), { method, headers, body })
    if (res.status === 401 && retry && !path.startsWith('/auth/')) {
      if (!tokens.getRefresh()) {
        // The refresh token is gone (e.g. cleared by another tab) but this tab still thinks it is
        // signed in: nothing can renew the session, so end it here instead of failing every query.
        if (accessToken !== null) { tokens.clear(); sessionLostHandler?.() }
        return res
      }
      const ok = await refreshSession()
      if (ok) return fetchWithAuth(method, path, init, false)
      // Rejected refresh: tokens already cleared and the handler notified. Transient failure: the
      // refresh token is kept and the caller gets this 401 as an ApiError it can offer to retry.
    }
    return res
  }

  async function request<T>(method: string, path: string, opts: FetchInit = {}): Promise<T> {
    let res: Response
    try {
      res = await fetchWithAuth(method, path, opts)
    } catch (e) {
      throw new ApiError(0, 'network', e instanceof Error ? e.message : 'network error')
    }
    if (!res.ok) throw await toError(res)
    if (res.status === 204) return undefined as T
    return (await res.json()) as T
  }

  return { tokens, refreshSession, waitForRefresh, fetchWithAuth, request }
}

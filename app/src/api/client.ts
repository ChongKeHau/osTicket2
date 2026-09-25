import type { ApiErrorBody, Session } from './types'

export const REFRESH_KEY = 'ticket.refresh_token'
const BASE = '/api/v1'

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

let accessToken: string | null = null
let refreshing: Promise<boolean> | null = null
let sessionLostHandler: (() => void) | null = null

function safeStorage<T>(fn: () => T, fallback: T): T {
  try { return fn() } catch { return fallback }
}

export const tokens = {
  get access(): string | null { return accessToken },
  getRefresh(): string | null { return safeStorage(() => localStorage.getItem(REFRESH_KEY), null) },
  setSession(s: Session): void {
    accessToken = s.access_token
    safeStorage(() => localStorage.setItem(REFRESH_KEY, s.refresh_token), undefined)
  },
  clear(): void {
    accessToken = null
    safeStorage(() => localStorage.removeItem(REFRESH_KEY), undefined)
  },
  setOnSessionLost(fn: (() => void) | null): void { sessionLostHandler = fn },
}

export type Query = Record<string, string | number | boolean | undefined | null>

function buildUrl(path: string, query?: Query): string {
  const url = new URL(BASE + path, window.location.origin)
  if (query) {
    for (const [k, v] of Object.entries(query)) {
      if (v === undefined || v === null || v === '') continue
      url.searchParams.set(k, String(v))
    }
  }
  return url.toString()
}

export async function refreshSession(): Promise<boolean> {
  if (refreshing) return refreshing
  const raw = tokens.getRefresh()
  if (!raw) return false
  refreshing = (async () => {
    try {
      const res = await fetch(buildUrl('/auth/refresh'), {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ refresh_token: raw }),
      })
      if (!res.ok) throw new Error('refresh failed')
      tokens.setSession((await res.json()) as Session)
      return true
    } catch {
      tokens.clear()
      sessionLostHandler?.()
      return false
    } finally {
      refreshing = null
    }
  })()
  return refreshing
}

export interface FetchInit { body?: unknown; formData?: FormData; query?: Query; headers?: Record<string, string> }

export async function fetchWithAuth(method: string, path: string, init: FetchInit = {}, retry = true): Promise<Response> {
  const headers: Record<string, string> = { Accept: 'application/json', ...init.headers }
  if (accessToken) headers['Authorization'] = `Bearer ${accessToken}`
  let body: BodyInit | undefined
  if (init.formData) body = init.formData
  else if (init.body !== undefined) { headers['Content-Type'] = 'application/json'; body = JSON.stringify(init.body) }
  const res = await fetch(buildUrl(path, init.query), { method, headers, body })
  if (res.status === 401 && retry && !path.startsWith('/auth/')) {
    const ok = await refreshSession()
    if (ok) return fetchWithAuth(method, path, init, false)
  }
  return res
}

async function toError(res: Response): Promise<ApiError> {
  try {
    const data = (await res.json()) as Partial<ApiErrorBody>
    if (data.error && typeof data.error.code === 'string') {
      return new ApiError(res.status, data.error.code, data.error.message ?? res.statusText, data.error.fields ?? {})
    }
  } catch { /* not JSON */ }
  return new ApiError(res.status, 'network', `request failed with status ${res.status}`)
}

export async function request<T>(method: string, path: string, opts: FetchInit = {}): Promise<T> {
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

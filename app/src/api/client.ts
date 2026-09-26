import { createSessionStore } from './sessionStore'

export { ApiError, type FetchInit, type Query } from './sessionStore'

export const REFRESH_KEY = 'ticket.refresh_token'

const staff = createSessionStore({ storageKey: REFRESH_KEY, base: '/api/v1', refreshPath: '/auth/refresh' })

export const tokens = staff.tokens
export const refreshSession = staff.refreshSession
export const waitForRefresh = staff.waitForRefresh
export const fetchWithAuth = staff.fetchWithAuth
export const request = staff.request

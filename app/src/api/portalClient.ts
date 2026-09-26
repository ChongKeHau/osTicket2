import { createSessionStore } from './sessionStore'
import type { PortalSession } from './types'

export const PORTAL_REFRESH_KEY = 'ticket.portal_refresh_token'

export const portal = createSessionStore<PortalSession>({ storageKey: PORTAL_REFRESH_KEY, base: '/api/v1/portal', refreshPath: '/auth/refresh' })

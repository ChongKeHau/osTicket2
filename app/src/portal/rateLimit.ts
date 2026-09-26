import { ApiError } from '../api/sessionStore'

/**
 * The portal's wording for a 429. The API puts `retry_after` (seconds, as a string) in the
 * envelope's fields; it is shown as whole minutes, rounded up, and never less than one.
 * Returns null for any other error so callers can fall back to their own message.
 */
export function rateLimitMessage(err: unknown): string | null {
  if (!(err instanceof ApiError) || err.status !== 429) return null
  const seconds = Number(err.fields.retry_after)
  const minutes = Number.isFinite(seconds) ? Math.max(1, Math.ceil(seconds / 60)) : 1
  return `Too many attempts, try again in ${minutes} minutes`
}

import { ApiError } from '../api/sessionStore'
import { rateLimitMessage } from './rateLimit'

const limited = (retryAfter?: string) =>
  new ApiError(429, 'rate_limited', 'too many attempts', retryAfter === undefined ? {} : { retry_after: retryAfter })

it('turns retry_after seconds into whole minutes, rounding up', () => {
  expect(rateLimitMessage(limited('600'))).toBe('Too many attempts, try again in 10 minutes')
  expect(rateLimitMessage(limited('61'))).toBe('Too many attempts, try again in 2 minutes')
})

it('never says less than one minute', () => {
  expect(rateLimitMessage(limited('0'))).toBe('Too many attempts, try again in 1 minutes')
  expect(rateLimitMessage(limited())).toBe('Too many attempts, try again in 1 minutes')
  expect(rateLimitMessage(limited('soon'))).toBe('Too many attempts, try again in 1 minutes')
})

it('is null for anything that is not a 429', () => {
  expect(rateLimitMessage(new ApiError(401, 'unauthorized', 'nope'))).toBeNull()
  expect(rateLimitMessage(new Error('boom'))).toBeNull()
  expect(rateLimitMessage(null)).toBeNull()
})

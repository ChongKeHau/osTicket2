import { http, HttpResponse } from 'msw'
import { REFRESH_KEY } from '../api/client'
import { adminFixtures, adminProfileFixture, referenceFixtures } from './fixtures'
import { server } from './setup'

/** Call in beforeEach: signs the test in as an admin and serves the larger admin fixtures. */
export function signInAsAdmin(): void {
  localStorage.setItem(REFRESH_KEY, 'refresh-1')
  server.use(
    http.get('/api/v1/me', () => HttpResponse.json(adminProfileFixture)),
    http.get('/api/v1/departments', () => HttpResponse.json({ items: adminFixtures.departments })),
    http.get('/api/v1/topics', () => HttpResponse.json({ items: referenceFixtures.topics })),
    http.get('/api/v1/staff', () => HttpResponse.json({ items: adminFixtures.staff })),
  )
}

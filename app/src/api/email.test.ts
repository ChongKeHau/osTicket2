import { http, HttpResponse } from 'msw'
import { server } from '../test/setup'
import { listOutbox, retryOutbox } from './email'

test('listOutbox sends status and paging as query params; retryOutbox POSTs to the row path', async () => {
  const urls: string[] = []
  server.use(
    http.get('/api/v1/email/outbox', ({ request }) => {
      urls.push(new URL(request.url).search)
      return HttpResponse.json({ items: [], page: 1, page_size: 25, total: 0 })
    }),
    http.post('/api/v1/email/outbox/:id/retry', ({ request, params }) => {
      urls.push(`${request.method} ${new URL(request.url).pathname} ${String(params.id)}`)
      return HttpResponse.json({ id: Number(params.id), status: 'pending' })
    }),
  )
  await listOutbox({ status: 'failed', page: 1, page_size: 25 })
  await listOutbox({ status: undefined, page: 2, page_size: 25 })
  expect(await retryOutbox(1)).toEqual({ id: 1, status: 'pending' })
  expect(urls).toEqual(['?status=failed&page=1&page_size=25', '?page=2&page_size=25', 'POST /api/v1/email/outbox/1/retry 1'])
})

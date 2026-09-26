import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { useState } from 'react'
import { uploadPortalFile } from '../api/portal'
import { server } from '../test/setup'
import { FileUpload, type PendingFile } from './FileUpload'

it('uploads through the given upload function instead of the staff endpoint', async () => {
  const hits: string[] = []
  server.use(
    http.post('/api/v1/files', () => { hits.push('staff'); return HttpResponse.json({ id: 1, name: 'a.txt', mime: 'text/plain', size: 3 }, { status: 201 }) }),
    http.post('/api/v1/portal/files', () => { hits.push('portal'); return HttpResponse.json({ id: 42, name: 'a.txt', mime: 'text/plain', size: 3 }, { status: 201 }) }),
  )
  let latest: PendingFile[] = []
  function Harness() {
    const [pending, setPending] = useState<PendingFile[]>([])
    latest = pending
    return <FileUpload pending={pending} onChange={setPending} inputId="f" upload={uploadPortalFile} />
  }
  render(<Harness />)
  await userEvent.upload(screen.getByLabelText(/attach files/i), new File(['abc'], 'a.txt', { type: 'text/plain' }))
  await vi.waitFor(() => expect(latest[0]?.status).toBe('done'))
  expect(latest[0]?.fileId).toBe(42)
  expect(hits).toEqual(['portal'])
})

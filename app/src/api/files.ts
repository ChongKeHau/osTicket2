import { ApiError, fetchWithAuth, request } from './client'
import type { FileInfo } from './types'

export function uploadFile(file: File): Promise<FileInfo> {
  const fd = new FormData()
  fd.append('file', file, file.name)
  return request<FileInfo>('POST', '/files', { formData: fd })
}

export async function downloadFile(id: number, name: string): Promise<void> {
  const res = await fetchWithAuth('GET', `/files/${id}`)
  if (!res.ok) throw new ApiError(res.status, res.status === 404 ? 'not_found' : 'network', 'download failed')
  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  document.body.appendChild(a)
  a.click()
  a.remove()
  // Some browsers start the download asynchronously; revoking right away can cancel it.
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

import { portal } from './portalClient'
import { ApiError } from './sessionStore'
import type {
  FileInfo, ListResponse, OpenTicketInput, PortalMe, PortalProfile, PortalReference, PortalSession, PortalTicket, PortalTicketRow,
} from './types'

export function getReference(): Promise<PortalReference> {
  return portal.request<PortalReference>('GET', '/reference')
}

export function openTicket(input: OpenTicketInput): Promise<{ id: number; number: string }> {
  return portal.request<{ id: number; number: string }>('POST', '/tickets', { body: input })
}

/** Portal uploads carry an access token; send it back in `file_tokens` beside the id in `file_ids`. */
export function uploadPortalFile(file: File): Promise<FileInfo & { token?: string }> {
  const fd = new FormData()
  fd.append('file', file, file.name)
  return portal.request<FileInfo & { token?: string }>('POST', '/files', { formData: fd })
}

export async function login(email: string, password: string): Promise<PortalSession> {
  const s = await portal.request<PortalSession>('POST', '/auth/login', { body: { email, password } })
  portal.tokens.setSession(s)
  return s
}

export function requestLink(email: string): Promise<void> {
  return portal.request<void>('POST', '/auth/link', { body: { email } })
}

/** Redeems an emailed one-time token; the response's `kind` says what it was issued for. */
export async function exchange(token: string): Promise<PortalSession> {
  const s = await portal.request<PortalSession>('POST', '/auth/exchange', { body: { token } })
  portal.tokens.setSession(s)
  return s
}

/** Creates an unverified account; the emailed confirm link then lets the customer set a password. */
export function register(input: { email: string; name: string }): Promise<void> {
  return portal.request<void>('POST', '/auth/register', { body: input })
}

export function requestReset(email: string): Promise<void> {
  return portal.request<void>('POST', '/auth/reset', { body: { email } })
}

export function requestAccess(email: string, number: string): Promise<void> {
  return portal.request<void>('POST', '/access', { body: { email, number } })
}

export async function logout(): Promise<void> {
  const refresh = portal.tokens.getRefresh()
  try {
    if (refresh) await portal.request<void>('POST', '/auth/logout', { body: { refresh_token: refresh } })
  } finally {
    portal.tokens.clear()
  }
}

export function listMyTickets(f: { state?: 'open' | 'closed'; page: number; page_size: number }): Promise<ListResponse<PortalTicketRow>> {
  return portal.request<ListResponse<PortalTicketRow>>('GET', '/tickets', { query: { ...f } })
}

export function getMyTicket(id: number): Promise<PortalTicket> {
  return portal.request<PortalTicket>('GET', `/tickets/${id}`)
}

export function replyTicket(id: number, input: { body: string; format: 'text' | 'html'; file_ids?: number[]; file_tokens?: string[] }): Promise<void> {
  return portal.request<void>('POST', `/tickets/${id}/reply`, { body: input })
}

export function closeTicket(id: number): Promise<PortalTicket> {
  return portal.request<PortalTicket>('POST', `/tickets/${id}/close`)
}

export function reopenTicket(id: number): Promise<PortalTicket> {
  return portal.request<PortalTicket>('POST', `/tickets/${id}/reopen`)
}

export function getMe(): Promise<PortalMe> {
  return portal.request<PortalMe>('GET', '/me')
}

export function updateMe(input: { name: string }): Promise<PortalProfile> {
  return portal.request<PortalProfile>('PATCH', '/me', { body: input })
}

export function setPassword(input: { password: string; current_password?: string }): Promise<void> {
  return portal.request<void>('POST', '/me/password', { body: input })
}

export async function downloadPortalFile(ticketId: number, fileId: number, name: string): Promise<void> {
  const res = await portal.fetchWithAuth('GET', `/tickets/${ticketId}/files/${fileId}`)
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

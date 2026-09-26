import { request } from './client'
import type { EmailTemplate, InboundItem, ListResponse, OutboxItem, TemplateInput } from './types'

export async function listEmailTemplates(): Promise<EmailTemplate[]> {
  return (await request<{ items: EmailTemplate[] }>('GET', '/email/templates')).items
}

export function updateEmailTemplate(key: string, input: TemplateInput): Promise<EmailTemplate> {
  return request('PATCH', `/email/templates/${encodeURIComponent(key)}`, { body: input })
}

export function listOutbox(f: { status?: string; page: number; page_size: number }): Promise<ListResponse<OutboxItem>> {
  return request('GET', '/email/outbox', { query: f })
}

export function retryOutbox(id: number): Promise<{ id: number; status: string }> {
  return request('POST', `/email/outbox/${id}/retry`)
}

export function listInbound(f: { page: number; page_size: number }): Promise<ListResponse<InboundItem>> {
  return request('GET', '/email/inbound', { query: f })
}

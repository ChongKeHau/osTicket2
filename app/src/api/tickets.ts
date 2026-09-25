import { request } from './client'
import type { CreateTicketInput, Entry, Event, ListFilter, ListResponse, NoteInput, ReplyInput, Thread, Ticket, UpdateTicketInput } from './types'

export function listTickets(f: ListFilter): Promise<ListResponse<Ticket>> {
  return request('GET', '/tickets', { query: { ...f } })
}
export function getTicket(id: number): Promise<Ticket> { return request('GET', `/tickets/${id}`) }
export function createTicket(input: CreateTicketInput): Promise<Ticket> { return request('POST', '/tickets', { body: input }) }
export function updateTicket(id: number, input: UpdateTicketInput): Promise<Ticket> { return request('PATCH', `/tickets/${id}`, { body: input }) }
export function reply(id: number, input: ReplyInput): Promise<Entry> { return request('POST', `/tickets/${id}/reply`, { body: input }) }
export function note(id: number, input: NoteInput): Promise<Entry> { return request('POST', `/tickets/${id}/notes`, { body: input }) }
export function getThread(id: number, after = 0, limit = 50): Promise<Thread> {
  return request('GET', `/tickets/${id}/thread`, { query: { after: after || undefined, limit } })
}
export function setStatus(id: number, statusId: number): Promise<Ticket> { return request('POST', `/tickets/${id}/status`, { body: { status_id: statusId } }) }
export function assign(id: number, staffId: number | null): Promise<Ticket> { return request('POST', `/tickets/${id}/assign`, { body: { staff_id: staffId } }) }
export function transfer(id: number, deptId: number): Promise<Ticket> { return request('POST', `/tickets/${id}/transfer`, { body: { dept_id: deptId } }) }
export async function getEvents(id: number): Promise<Event[]> {
  return (await request<{ items: Event[] }>('GET', `/tickets/${id}/events`)).items
}

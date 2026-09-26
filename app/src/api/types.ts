export interface Ref { id: number; name: string }
export type TicketState = 'open' | 'resolved' | 'closed'

export interface Ticket {
  id: number; number: string; subject: string; status: Ref; state: TicketState
  department: Ref; topic: Ref | null; priority: Ref; assignee: Ref | null
  requester_name: string; requester_email: string; source: string; is_answered: boolean
  due_at: string | null; closed_at: string | null; last_message_at: string; last_response_at: string | null
  extra: Record<string, unknown>; created_at: string; updated_at: string
}

export interface ListResponse<T> { items: T[]; page: number; page_size: number; total: number }

export interface AttachmentRef { file_id: number; name: string; mime: string; size: number }
export type EntryType = 'message' | 'response' | 'note'
export interface Entry {
  id: number; ticket_id: number; type: EntryType; staff_id: number | null; poster: string
  /** The end user who wrote a customer message, when known. */
  user_id?: number | null
  title: string | null; body: string; format: 'html' | 'text'; parent_id: number | null
  attachments: AttachmentRef[]; created_at: string
}
export interface Thread { items: Entry[]; next_after: number | null }
export interface Event { id: number; ticket_id: number; staff: Ref | null; kind: string; data: Record<string, unknown>; created_at: string }

export interface StaffProfile { id: number; username: string; email: string; first_name: string; last_name: string; is_admin: boolean; department_ids: number[] }
export interface Session { access_token: string; refresh_token: string; expires_in: number; staff: StaffProfile }

export interface Priority { id: number; name: string; urgency: number; color: string }
export interface Status { id: number; name: string; state: TicketState; sort_order: number }
export interface Department { id: number; name: string; is_public: boolean; manager_id: number | null }
export interface Topic { id: number; name: string; dept_id: number | null; priority_id: number | null; is_active: boolean; sort_order: number }
export interface Staff { id: number; username: string; email: string; first_name: string; last_name: string; is_admin: boolean; is_active: boolean; primary_dept_id: number; department_ids: number[] }

export interface DepartmentInput { name: string; is_public: boolean; manager_id: number | null }
export interface TopicInput { name: string; dept_id: number | null; priority_id: number | null; is_active: boolean; sort_order: number }
export interface CreateStaffInput {
  username: string; email: string; password: string; first_name: string; last_name: string
  is_admin: boolean; primary_dept_id: number; department_ids: number[]
}
export interface UpdateStaffInput {
  email?: string; first_name?: string; last_name?: string; is_admin?: boolean; is_active?: boolean
  primary_dept_id?: number; department_ids?: number[]
}

export interface FileInfo { id: number; name: string; mime: string; size: number }

export interface ListFilter {
  state?: TicketState; status?: number; dept_id?: number; assigned_to?: string; q?: string
  sort?: string; page: number; page_size: number
}
export interface CreateTicketInput {
  subject: string; message: string; message_format?: 'html' | 'text'; requester_name?: string; requester_email: string
  dept_id?: number; topic_id?: number; priority_id?: number; source?: string; due_at?: string | null; file_ids?: number[]
}
export interface UpdateTicketInput {
  subject?: string; priority_id?: number; topic_id?: number | null; due_at?: string | null
  requester_name?: string; requester_email?: string
}
export interface ReplyInput { body: string; format: 'html' | 'text'; status_id?: number; file_ids?: number[] }
export interface NoteInput { title?: string; body: string; format: 'html' | 'text'; file_ids?: number[] }

export interface EmailTemplate { key: string; subject: string; body_html: string; body_text: string; updated_at: string }
export interface TemplateInput { subject?: string; body_html?: string; body_text?: string }
export type OutboxStatus = 'pending' | 'sent' | 'failed'
export interface OutboxItem {
  id: number; ticket_id: number | null; entry_id: number | null; template_key: string; to_address: string; to_name: string
  subject: string; status: OutboxStatus; attempts: number; last_error: string | null; next_attempt_at: string
  sent_at: string | null; created_at: string
}
/** One outbox row with its rendered bodies (GET /email/outbox/:id). */
export interface OutboxDetail extends OutboxItem { body_text: string; body_html: string }
export type InboundOutcome = 'created' | 'replied' | 'ignored'
export interface InboundItem {
  id: number; message_id: string; from_address: string; from_name: string; subject: string
  ticket_id: number | null; entry_id: number | null; outcome: InboundOutcome; reason: string; received_at: string
}

export interface ApiErrorBody { error: { code: string; message: string; fields?: Record<string, string> } }

export interface DashboardPoint { date: string; opened: number; assigned: number; closed: number; reopened: number }
/** One breakdown row; `id` is null for the "— none —" topic and "— system —" staff rows. */
export interface DashboardRow { id: number | null; name: string; opened: number; assigned: number; closed: number; reopened: number }
export interface DashboardStats {
  start: string; period: number; series: DashboardPoint[]
  by_department: DashboardRow[]; by_topic: DashboardRow[]; by_staff: DashboardRow[]
}

// Customer portal (`/api/v1/portal`).
export interface PortalProfile { id: number; email: string; name: string; verified: boolean; has_password: boolean }
/** What an emailed token was issued for; only token exchanges carry it, and the page routes by it. */
/** `GET /me`: the profile plus the ticket a guest session is scoped to (null for an account). */
export interface PortalMe extends PortalProfile { ticket_id: number | null }
export type PortalTokenKind = 'confirm' | 'signin' | 'access' | 'reset'
export interface PortalSession {
  access_token: string; refresh_token: string; expires_in: number; user: PortalProfile
  /** Set for a guest session scoped to one ticket; null for a full account session. */
  ticket_id: number | null
  kind?: PortalTokenKind
}
export interface PortalReference { site_name: string; departments: Ref[]; topics: Ref[] }
export interface OpenTicketInput {
  name: string; email: string; subject: string; message: string; format: 'text' | 'html'
  topic_id?: number; dept_id?: number; file_ids?: number[]; file_tokens?: string[]
}
export interface PortalTicketRow {
  id: number; number: string; subject: string; status: Ref; state: TicketState; department: string
  created_at: string; last_message_at: string; closed_at: string | null
}
export interface PortalEntry {
  id: number; type: 'message' | 'response'; poster: string; body: string; format: 'html' | 'text'
  created_at: string; attachments: AttachmentRef[]
}
export interface PortalTicket extends PortalTicketRow { topic: string | null; updated_at: string; entries: PortalEntry[] }

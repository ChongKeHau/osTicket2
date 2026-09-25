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

export interface ApiErrorBody { error: { code: string; message: string; fields?: Record<string, string> } }

import type { Department, Entry, Event, Priority, Session, Staff, StaffProfile, Status, Ticket, Topic } from '../api/types'

export const staffProfileFixture: StaffProfile = {
  id: 1, username: 'agent', email: 'agent@example.test', first_name: 'Ann', last_name: 'Agent',
  is_admin: false, department_ids: [1],
}

export const sessionFixture: Session = {
  access_token: 'access-1', refresh_token: 'refresh-1', expires_in: 900, staff: staffProfileFixture,
}

export const ticketFixture: Ticket = {
  id: 7, number: '000007', subject: 'Printer on fire', status: { id: 1, name: 'Open' }, state: 'open',
  department: { id: 1, name: 'Support' }, topic: { id: 1, name: 'General Inquiry' },
  priority: { id: 2, name: 'normal' }, assignee: null, requester_name: 'Pat', requester_email: 'pat@example.test',
  source: 'phone', is_answered: false, due_at: null, closed_at: null,
  last_message_at: '2026-09-25T10:00:00Z', last_response_at: null, extra: {},
  created_at: '2026-09-25T10:00:00Z', updated_at: '2026-09-25T10:00:00Z',
}

export const entryFixtures: Entry[] = [
  { id: 1, ticket_id: 7, type: 'message', staff_id: null, poster: 'Pat', title: null, body: '<p>Help</p>',
    format: 'html', parent_id: null, attachments: [], created_at: '2026-09-25T10:00:00Z' },
  { id: 2, ticket_id: 7, type: 'response', staff_id: 1, poster: 'Ann Agent', title: null, body: 'On it',
    format: 'text', parent_id: null, attachments: [{ file_id: 3, name: 'log.txt', mime: 'text/plain', size: 12 }],
    created_at: '2026-09-25T10:05:00Z' },
]

export const eventFixtures: Event[] = [
  { id: 1, ticket_id: 7, staff: { id: 1, name: 'Ann Agent' }, kind: 'created', data: { number: '000007' }, created_at: '2026-09-25T10:00:00Z' },
]

export const referenceFixtures = {
  priorities: [{ id: 1, name: 'low', urgency: 1, color: '' }, { id: 2, name: 'normal', urgency: 2, color: '' }, { id: 3, name: 'high', urgency: 3, color: '' }] as Priority[],
  statuses: [{ id: 1, name: 'Open', state: 'open', sort_order: 1 }, { id: 2, name: 'Resolved', state: 'resolved', sort_order: 2 }, { id: 3, name: 'Closed', state: 'closed', sort_order: 3 }] as Status[],
  departments: [{ id: 1, name: 'Support', is_public: true, manager_id: null }, { id: 2, name: 'Billing', is_public: true, manager_id: null }] as Department[],
  topics: [{ id: 1, name: 'General Inquiry', dept_id: 1, priority_id: 2, is_active: true, sort_order: 1 }, { id: 2, name: 'Refunds', dept_id: 2, priority_id: 3, is_active: true, sort_order: 2 }] as Topic[],
  staff: [
    { id: 1, username: 'agent', email: 'agent@example.test', first_name: 'Ann', last_name: 'Agent', is_admin: false, is_active: true, primary_dept_id: 1, department_ids: [1] },
    { id: 2, username: 'bob', email: 'bob@example.test', first_name: 'Bob', last_name: 'Billing', is_admin: false, is_active: true, primary_dept_id: 2, department_ids: [2] },
    { id: 3, username: 'root', email: 'root@example.test', first_name: 'Root', last_name: 'Admin', is_admin: true, is_active: true, primary_dept_id: 1, department_ids: [1] },
  ] as Staff[],
}

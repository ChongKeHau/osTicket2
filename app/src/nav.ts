import type { SubNavItem } from './ui/SubNav'

export const AGENT_TABS = [
  { label: 'Dashboard', to: '/dashboard' },
  { label: 'Tickets', to: '/tickets' },
]

export const ADMIN_TABS = [
  { label: 'Dashboard', to: '/dashboard' },
  { label: 'Departments', to: '/admin/departments' },
  { label: 'Help Topics', to: '/admin/topics' },
  { label: 'Staff', to: '/admin/staff' },
  { label: 'Email', to: '/admin/email' },
]

export const TICKET_QUEUES = [
  { label: 'Open', search: '?state=open' },
  { label: 'My Tickets', search: '?state=open&assigned_to=me' },
  { label: 'Unassigned', search: '?state=open&assigned_to=none' },
  { label: 'Closed', search: '?state=closed' },
  { label: 'All', search: '' },
] as const

/** Sub-nav items for the Tickets tab; the active one is the queue whose
 *  state and assignment match the current filters (page/sort/q/status/dept_id ignored). */
export function ticketSubNav(current: URLSearchParams): SubNavItem[] {
  const norm = (p: URLSearchParams) => {
    const q = new URLSearchParams()
    for (const k of ['state', 'assigned_to']) { const v = p.get(k); if (v) q.set(k, v) }
    return q.toString()
  }
  const here = norm(current)
  return TICKET_QUEUES.map((qd) => ({ label: qd.label, to: `/tickets${qd.search}`, active: norm(new URLSearchParams(qd.search)) === here }))
}

export function queueTitle(current: URLSearchParams): string {
  if (current.get('q')) return 'Search Results'
  const hit = ticketSubNav(current).find((i) => i.active)
  return hit ? (hit.label === 'All' ? 'All Tickets' : hit.label.endsWith('Tickets') ? hit.label : `${hit.label} Tickets`) : 'Tickets'
}

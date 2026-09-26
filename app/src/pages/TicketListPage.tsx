import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useMemo, useState, type FormEvent } from 'react'
import { useSearchParams } from 'react-router-dom'
import { listTickets } from '../api/tickets'
import type { Ticket } from '../api/types'
import { useReferenceData } from '../hooks/useReferenceData'
import { useTicketFilters } from '../hooks/useTicketFilters'
import { formatDate, relativeTime } from '../lib/format'
import { queueTitle, ticketSubNav } from '../nav'
import { useSubNav } from '../ui/AppShell'
import { Badge } from '../ui/Badge'
import { Banner, errorMessage } from '../ui/Banner'
import { Button, LinkButton } from '../ui/Button'
import { ListTable, type Column } from '../ui/ListTable'
import { Pagination } from '../ui/Pagination'
import { StickyBar } from '../ui/StickyBar'
import s from './TicketListPage.module.css'

const newTicket = <LinkButton to="/tickets/new" variant="add">New Ticket</LinkButton>

export function TicketListPage() {
  const [params] = useSearchParams()
  const { filter, set } = useTicketFilters()
  const { departments, statuses, priorities } = useReferenceData()
  useSubNav(useMemo(() => ticketSubNav(params), [params]), newTicket)
  const [q, setQ] = useState(filter.q ?? '')

  const list = useQuery({ queryKey: ['tickets', filter], queryFn: () => listTickets(filter), placeholderData: keepPreviousData })

  const columns: Column<Ticket>[] = [
    { key: 'number', label: 'Number', width: '9em', render: (t) => <span className={s.number}>{t.number}</span> },
    { key: 'created', label: 'Date', sortKey: 'created_at', width: '8em', render: (t) => <span title={t.created_at}>{formatDate(t.created_at)}</span> },
    { key: 'subject', label: 'Subject', cellClass: (t) => (t.is_answered ? undefined : 'unanswered'), render: (t) => t.subject },
    { key: 'from', label: 'From', render: (t) => t.requester_name || t.requester_email },
    { key: 'priority', label: 'Priority', sortKey: 'priority', width: '7em', render: (t) => <Badge color={priorities.find((p) => p.id === t.priority.id)?.color}>{t.priority.name}</Badge> },
    { key: 'dept', label: 'Department', render: (t) => t.department.name },
    { key: 'assignee', label: 'Assigned To', render: (t) => t.assignee?.name ?? <span className="muted">—</span> },
    { key: 'last', label: 'Last Message', sortKey: 'last_message_at', width: '8em', render: (t) => <span title={t.last_message_at}>{relativeTime(t.last_message_at)}</span> },
  ]

  const onSearch = (e: FormEvent) => { e.preventDefault(); set({ q: q.trim() || undefined, page: 1 }) }

  return (
    <>
      <form className={s.search} onSubmit={onSearch} role="search">
        <input type="search" value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search tickets" aria-label="Search tickets" />
        <Button type="submit">Search</Button>
      </form>
      <StickyBar
        title={queueTitle(params)}
        count={list.data?.total}
        actions={
          <>
            <select aria-label="Department" value={filter.dept_id ?? ''} onChange={(e) => set({ dept_id: e.target.value ? Number(e.target.value) : undefined, page: 1 })}>
              <option value="">All departments</option>
              {departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
            </select>
            <select aria-label="Status" value={filter.status ?? ''} onChange={(e) => set({ status: e.target.value ? Number(e.target.value) : undefined, page: 1 })}>
              <option value="">Any status</option>
              {statuses.map((st) => <option key={st.id} value={st.id}>{st.name}</option>)}
            </select>
          </>
        }
      />
      {list.error && <Banner level="error">{errorMessage(list.error)}</Banner>}
      <ListTable
        columns={columns}
        rows={list.data?.items ?? []}
        rowKey={(t) => t.id}
        sort={filter.sort}
        onSort={(sort) => set({ sort, page: 1 })}
        rowHref={(t) => `/tickets/${t.id}`}
        empty={list.isPending ? 'Loading…' : 'No tickets found'}
        footer={list.data && <Pagination page={list.data.page} pageSize={list.data.page_size} total={list.data.total} onPage={(page) => set({ page })} />}
      />
    </>
  )
}

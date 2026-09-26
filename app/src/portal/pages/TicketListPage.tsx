import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import { listMyTickets } from '../../api/portal'
import type { PortalTicketRow } from '../../api/types'
import { formatDate, relativeTime } from '../../lib/format'
import { Banner, errorMessage } from '../../ui/Banner'
import { ListTable, type Column } from '../../ui/ListTable'
import { Pagination } from '../../ui/Pagination'
import { usePortalSubNav } from '../PortalShell'
import s from './TicketPage.module.css'

const PAGE_SIZE = 25

const columns: Column<PortalTicketRow>[] = [
  { key: 'number', label: 'Number', width: '9em', render: (t) => <span className={s.number}>{t.number}</span> },
  { key: 'created', label: 'Date', width: '8em', render: (t) => <span title={t.created_at}>{formatDate(t.created_at)}</span> },
  { key: 'subject', label: 'Subject', render: (t) => t.subject },
  { key: 'dept', label: 'Department', render: (t) => t.department },
  { key: 'status', label: 'Status', width: '8em', render: (t) => t.status.name },
  { key: 'last', label: 'Last Message', width: '8em', render: (t) => <span title={t.last_message_at}>{relativeTime(t.last_message_at)}</span> },
]

/** The signed-in customer's tickets, Open or Closed (`?state=`), newest activity first. */
export function TicketListPage() {
  const [params, setParams] = useSearchParams()
  const state = params.get('state') === 'closed' ? 'closed' : 'open'
  const page = Math.max(1, Number(params.get('page')) || 1)
  usePortalSubNav([
    { label: 'Open', to: '/portal/tickets?state=open', active: state === 'open' },
    { label: 'Closed', to: '/portal/tickets?state=closed', active: state === 'closed' },
  ])

  const list = useQuery({
    queryKey: ['portal', 'tickets', { state, page }],
    queryFn: () => listMyTickets({ state, page, page_size: PAGE_SIZE }),
    placeholderData: keepPreviousData,
  })

  return (
    <>
      <h2 className={s.title}>{state === 'closed' ? 'Closed Tickets' : 'Open Tickets'}</h2>
      {list.error && <Banner level="error">{errorMessage(list.error)}</Banner>}
      <ListTable
        columns={columns}
        rows={list.data?.items ?? []}
        rowKey={(t) => t.id}
        rowHref={(t) => `/portal/tickets/${t.id}`}
        empty={list.isPending ? 'Loading…' : `You have no ${state} tickets`}
        footer={list.data && (
          <Pagination page={list.data.page} pageSize={list.data.page_size} total={list.data.total}
            onPage={(p) => setParams({ state, page: String(p) })} />
        )}
      />
    </>
  )
}

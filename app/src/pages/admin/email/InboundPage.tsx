import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { listInbound } from '../../../api/email'
import type { InboundItem, InboundOutcome } from '../../../api/types'
import { formatDateTime } from '../../../lib/format'
import { Badge } from '../../../ui/Badge'
import { Banner, errorMessage } from '../../../ui/Banner'
import { ListTable, type Column } from '../../../ui/ListTable'
import { Pagination } from '../../../ui/Pagination'
import { StickyBar } from '../../../ui/StickyBar'
import s from './email.module.css'
import { addressee, PAGE_SIZE, useEmailSubNav, useListParams } from './emailNav'

const OUTCOME_COLOR: Record<InboundOutcome, string | undefined> = { created: 'var(--accent)', replied: 'var(--green)', ignored: undefined }

const columns: Column<InboundItem>[] = [
  { key: 'received', label: 'Received', width: '12em', render: (r) => formatDateTime(r.received_at) },
  { key: 'from', label: 'From', render: (r) => addressee(r.from_name, r.from_address) },
  { key: 'subject', label: 'Subject', render: (r) => r.subject },
  { key: 'outcome', label: 'Outcome', render: (r) => <Badge color={OUTCOME_COLOR[r.outcome]}>{r.outcome}</Badge> },
  { key: 'ticket', label: 'Ticket', render: (r) => (r.ticket_id === null ? '—' : <Link to={`/tickets/${r.ticket_id}`}>#{r.ticket_id}</Link>) },
  { key: 'reason', label: 'Reason', render: (r) => r.reason },
]

export function InboundPage() {
  useEmailSubNav()
  const { page, update } = useListParams()
  const q = useQuery({
    queryKey: ['email', 'inbound', { page }],
    queryFn: () => listInbound({ page, page_size: PAGE_SIZE }),
    placeholderData: keepPreviousData,
  })
  const data = q.data
  return (
    <>
      <StickyBar title="Inbound Log" count={data?.total} />
      {q.error && <Banner level="error">{errorMessage(q.error)}</Banner>}
      <ListTable columns={columns} rows={data?.items ?? []} rowKey={(r) => r.id}
        empty={q.isLoading ? 'Loading…' : 'No inbound messages'}
        footer={data && (
          <div className={s.footer}>
            <span>{data.total} messages</span>
            <Pagination page={data.page} pageSize={data.page_size} total={data.total} onPage={(p) => update({ page: p === 1 ? null : String(p) })} />
          </div>
        )} />
    </>
  )
}

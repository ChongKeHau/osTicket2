import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { listOutbox, retryOutbox } from '../../../api/email'
import type { OutboxItem, OutboxStatus } from '../../../api/types'
import { formatDateTime, truncate } from '../../../lib/format'
import { Badge } from '../../../ui/Badge'
import { Banner, errorMessage } from '../../../ui/Banner'
import { useBanner } from '../../../ui/BannerContext'
import { Button } from '../../../ui/Button'
import { ListTable, type Column } from '../../../ui/ListTable'
import { Pagination } from '../../../ui/Pagination'
import { StickyBar } from '../../../ui/StickyBar'
import s from './email.module.css'
import { addressee, PAGE_SIZE, useEmailSubNav, useListParams } from './emailNav'

const STATUSES: OutboxStatus[] = ['pending', 'sent', 'failed']
const STATUS_COLOR: Record<OutboxStatus, string | undefined> = { pending: undefined, sent: 'var(--green)', failed: 'var(--danger)' }

export function OutboxPage() {
  useEmailSubNav()
  const { params, page, update } = useListParams()
  const status = params.get('status') ?? ''
  const qc = useQueryClient()
  const { flash, clear } = useBanner()
  const q = useQuery({
    queryKey: ['email', 'outbox', { status, page }],
    queryFn: () => listOutbox({ status: status || undefined, page, page_size: PAGE_SIZE }),
    placeholderData: keepPreviousData,
  })
  const retry = useMutation({
    mutationFn: retryOutbox,
    onSuccess: async () => {
      flash('notice', 'Queued for retry')
      await qc.invalidateQueries({ queryKey: ['email', 'outbox'] })
    },
  })

  const columns: Column<OutboxItem>[] = [
    { key: 'id', label: 'ID', width: '4em', align: 'right', render: (r) => String(r.id) },
    { key: 'ticket', label: 'Ticket', render: (r) => <Link to={`/tickets/${r.ticket_id}`}>#{r.ticket_id}</Link> },
    { key: 'to', label: 'To', render: (r) => addressee(r.to_name, r.to_address) },
    { key: 'subject', label: 'Subject', render: (r) => r.subject },
    { key: 'status', label: 'Status', render: (r) => <Badge color={STATUS_COLOR[r.status]}>{r.status}</Badge> },
    { key: 'attempts', label: 'Attempts', align: 'right', render: (r) => String(r.attempts) },
    { key: 'next', label: 'Next attempt', render: (r) => (r.status === 'pending' || r.status === 'failed' ? formatDateTime(r.next_attempt_at) : '') },
    { key: 'error', label: 'Last error', render: (r) => (r.last_error ? <span title={r.last_error}>{truncate(r.last_error, 60)}</span> : '') },
    {
      key: 'actions', label: '', align: 'right',
      render: (r) => (r.status === 'failed'
        ? <Button size="sm" disabled={retry.isPending} onClick={() => { clear(); retry.mutate(r.id) }}>Retry</Button>
        : null),
    },
  ]

  const data = q.data
  const statusSelect = (
    <select aria-label="Status" value={status} onChange={(e) => update({ status: e.target.value, page: null })}>
      <option value="">All</option>
      {STATUSES.map((st) => <option key={st} value={st}>{st}</option>)}
    </select>
  )

  return (
    <>
      <StickyBar title="Outbox" count={data?.total} actions={statusSelect} />
      {q.error && <Banner level="error">{errorMessage(q.error)}</Banner>}
      {retry.error && <Banner level="error">{errorMessage(retry.error)}</Banner>}
      <ListTable columns={columns} rows={data?.items ?? []} rowKey={(r) => r.id}
        empty={q.isLoading ? 'Loading…' : 'No outgoing messages'}
        footer={data && (
          <div className={s.footer}>
            <span>{data.total} messages</span>
            <Pagination page={data.page} pageSize={data.page_size} total={data.total} onPage={(p) => update({ page: p === 1 ? null : String(p) })} />
          </div>
        )} />
    </>
  )
}

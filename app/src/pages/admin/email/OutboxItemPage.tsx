import { useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { ApiError } from '../../../api/client'
import { getOutboxItem } from '../../../api/email'
import { LoadingScreen } from '../../../components/LoadingScreen'
import { parseId } from '../../../lib/forms'
import { formatDateTime } from '../../../lib/format'
import { Badge } from '../../../ui/Badge'
import { Banner, errorMessage } from '../../../ui/Banner'
import { FormTable } from '../../../ui/FormTable'
import { StickyBar } from '../../../ui/StickyBar'
import s from './email.module.css'
import { addressee, useEmailSubNav } from './emailNav'

/** One queued or sent message: its headers, the text body (the page's first `<pre>`), and the HTML source. */
export function OutboxItemPage() {
  useEmailSubNav()
  const id = parseId(useParams().id)
  const q = useQuery({ queryKey: ['email', 'outbox', 'item', id], queryFn: () => getOutboxItem(id as number), enabled: id !== null })
  if (id === null || (q.error instanceof ApiError && q.error.status === 404)) return <Banner level="error">Message not found.</Banner>
  if (q.isLoading) return <LoadingScreen />
  if (!q.data) return <Banner level="error">{errorMessage(q.error)}</Banner>
  const m = q.data
  return (
    <>
      <StickyBar title={`Outbox Message #${m.id}`} actions={<Link to="/admin/email/outbox">Back to Outbox</Link>} />
      <FormTable sections={[
        { title: 'Message', rows: [
          { label: 'To', control: addressee(m.to_name, m.to_address) },
          { label: 'Subject', control: m.subject },
          { label: 'Template', control: <code>{m.template_key}</code> },
          { label: 'Ticket', control: m.ticket_id === null ? '—' : <Link to={`/tickets/${m.ticket_id}`}>#{m.ticket_id}</Link> },
          { label: 'Status', control: <Badge>{m.status}</Badge> },
          { label: 'Attempts', control: String(m.attempts) },
          { label: 'Created', control: formatDateTime(m.created_at) },
          { label: 'Sent', control: m.sent_at ? formatDateTime(m.sent_at) : '—' },
          ...(m.last_error ? [{ label: 'Last error', control: m.last_error }] : []),
        ] },
        { title: 'Body', rows: [
          { label: 'Text body', control: <pre className={s.pre} aria-label="Text body">{m.body_text}</pre> },
          { label: 'HTML source', control: (
            <details>
              <summary>Show HTML</summary>
              <pre className={`${s.pre} ${s.mono}`} aria-label="HTML body">{m.body_html}</pre>
            </details>
          ) },
        ] },
      ]} />
    </>
  )
}

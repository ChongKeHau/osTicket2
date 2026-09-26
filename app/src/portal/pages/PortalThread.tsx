import { downloadPortalFile } from '../../api/portal'
import type { PortalEntry } from '../../api/types'
import { AttachmentList } from '../../components/AttachmentList'
import { formatDateTime, relativeTime } from '../../lib/format'
import { sanitizeHtml } from '../../lib/sanitize'
import s from './TicketPage.module.css'

/** A customer's view of the thread: their messages on the left, agent responses on the right. */
export function PortalThread({ ticketId, entries }: { ticketId: number; entries: PortalEntry[] }) {
  const download = (fileId: number, name: string) => downloadPortalFile(ticketId, fileId, name)
  return (
    <section className={s.thread} aria-label="Thread">
      {entries.length === 0 && <p className="muted">No messages yet.</p>}
      {entries.map((e) => (
        <article key={e.id} className={`${s.entry} ${s[e.type] ?? ''}`}>
          <header className={s.entryHeader}>
            <span className={s.poster}>{e.poster || 'Unknown'}</span>
            <time className={s.time} dateTime={e.created_at} title={relativeTime(e.created_at)}>{formatDateTime(e.created_at)}</time>
          </header>
          <div className={s.body}>
            {e.format === 'html'
              ? <div dangerouslySetInnerHTML={{ __html: sanitizeHtml(e.body) }} />
              : <pre className="pre">{e.body}</pre>}
          </div>
          {e.attachments.length > 0 && <div className={s.attachments}><AttachmentList attachments={e.attachments} download={download} /></div>}
        </article>
      ))}
    </section>
  )
}

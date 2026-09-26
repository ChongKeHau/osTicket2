import type { Entry } from '../api/types'
import { formatDateTime, relativeTime } from '../lib/format'
import { sanitizeHtml } from '../lib/sanitize'
import { Badge } from '../ui/Badge'
import { AttachmentList } from './AttachmentList'
import s from './ThreadEntry.module.css'

export function ThreadEntry({ entry }: { entry: Entry }) {
  return (
    <article className={`${s.entry} ${s[entry.type] ?? ''}`}>
      <header className={s.header}>
        <span className={s.poster}>{entry.poster || 'Unknown'}</span>
        {entry.type === 'note' && <Badge>Internal Note</Badge>}
        {entry.title && <em>{entry.title}</em>}
        <time className={s.time} dateTime={entry.created_at} title={relativeTime(entry.created_at)}>{formatDateTime(entry.created_at)}</time>
      </header>
      <div className={s.body}>
        {entry.format === 'html'
          ? <div dangerouslySetInnerHTML={{ __html: sanitizeHtml(entry.body) }} />
          : <pre className="pre">{entry.body}</pre>}
      </div>
      {entry.attachments.length > 0 && <div className={s.attachments}><AttachmentList attachments={entry.attachments} /></div>}
    </article>
  )
}

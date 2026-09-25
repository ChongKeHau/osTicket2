import type { Entry } from '../api/types'
import { formatDateTime } from '../lib/format'
import { sanitizeHtml } from '../lib/sanitize'
import { AttachmentList } from './AttachmentList'
import styles from './ThreadEntry.module.css'

const labels: Record<Entry['type'], string> = { message: 'Message', response: 'Response', note: 'Note' }

export function ThreadEntry({ entry }: { entry: Entry }) {
  return (
    <article className={`${styles.entry} ${styles[entry.type] ?? ''}`}>
      <header className={styles.head}>
        <span className={styles.badge}>{labels[entry.type]}</span>
        <strong>{entry.poster || 'Unknown'}</strong>
        <span className="muted">{formatDateTime(entry.created_at)}</span>
        {entry.title && <em>{entry.title}</em>}
      </header>
      {entry.format === 'html'
        ? <div dangerouslySetInnerHTML={{ __html: sanitizeHtml(entry.body) }} />
        : <pre className="pre">{entry.body}</pre>}
      <AttachmentList attachments={entry.attachments} />
    </article>
  )
}

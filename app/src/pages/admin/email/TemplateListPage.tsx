import { useQuery } from '@tanstack/react-query'
import { listEmailTemplates } from '../../../api/email'
import type { EmailTemplate } from '../../../api/types'
import { formatDateTime } from '../../../lib/format'
import { Banner, errorMessage } from '../../../ui/Banner'
import { ListTable, type Column } from '../../../ui/ListTable'
import { StickyBar } from '../../../ui/StickyBar'
import { useEmailSubNav } from './emailNav'

const columns: Column<EmailTemplate>[] = [
  { key: 'key', label: 'Key', render: (t) => t.key },
  { key: 'subject', label: 'Subject', render: (t) => t.subject },
  { key: 'updated', label: 'Updated', width: '14em', render: (t) => formatDateTime(t.updated_at) },
]

export function TemplateListPage() {
  useEmailSubNav()
  const q = useQuery({ queryKey: ['email', 'templates'], queryFn: listEmailTemplates })
  const rows = q.data ?? []
  return (
    <>
      <StickyBar title="Email Templates" count={q.data ? rows.length : undefined} />
      {q.error && <Banner level="error">{errorMessage(q.error)}</Banner>}
      <ListTable columns={columns} rows={rows} rowKey={(t) => t.key} rowHref={(t) => `/admin/email/templates/${encodeURIComponent(t.key)}`}
        empty={q.isLoading ? 'Loading…' : 'No email templates'} footer={<span>{rows.length} templates</span>} />
    </>
  )
}

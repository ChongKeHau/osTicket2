import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { useParams } from 'react-router-dom'
import { listEmailTemplates, updateEmailTemplate } from '../../../api/email'
import type { EmailTemplate, TemplateInput } from '../../../api/types'
import { LoadingScreen } from '../../../components/LoadingScreen'
import { changedFields, splitErrors } from '../../../lib/forms'
import { Banner, errorMessage } from '../../../ui/Banner'
import { useBanner } from '../../../ui/BannerContext'
import { FormActions } from '../../../ui/FormActions'
import { FormTable } from '../../../ui/FormTable'
import { StickyBar } from '../../../ui/StickyBar'
import { NotFoundPage } from '../../NotFoundPage'
import s from './email.module.css'
import { useEmailSubNav } from './emailNav'

const LIST = '/admin/email/templates'
const KNOWN = ['subject', 'body_html', 'body_text']

/** The exported fields of mail.Vars (gin/internal/mail/template.go). */
const VARIABLES: [string, string][] = [
  ['{{.Number}}', 'Ticket number'],
  ['{{.Subject}}', 'Ticket subject'],
  ['{{.Message}}', 'Message body as plain text'],
  ['{{.MessageHTML}}', 'Message body as HTML'],
  ['{{.RequesterName}}', 'Requester name'],
  ['{{.RequesterEmail}}', 'Requester email address'],
  ['{{.AgentName}}', 'Name of the replying agent'],
  ['{{.SiteName}}', 'Help desk name'],
  ['{{.Link}}', 'Link to the ticket'],
]

// Inline, not in the CSS module: the token test forbids font-family in modules.
const MONO = { fontFamily: 'var(--font-mono)' }

type Fields = Required<TemplateInput>
const toInput = (t: EmailTemplate): Fields => ({ subject: t.subject, body_html: t.body_html, body_text: t.body_text })

export function TemplateFormPage() {
  const { key = '' } = useParams()
  useEmailSubNav()
  const q = useQuery({ queryKey: ['email', 'templates'], queryFn: listEmailTemplates })
  if (q.isLoading) return <LoadingScreen />
  // A failed background refetch keeps the last data, so only a first-load failure replaces the form.
  if (q.error && !q.data) return <Banner level="error">{errorMessage(q.error)}</Banner>
  const record = q.data?.find((t) => t.key === key)
  if (!record) return <NotFoundPage />
  return <TemplateForm key={record.key} record={record} />
}

function TemplateForm({ record }: { record: EmailTemplate }) {
  const qc = useQueryClient()
  const { flash, clear } = useBanner()
  const initial = toInput(record)
  const [form, setForm] = useState<Fields>(initial)
  const set = (k: keyof Fields, v: string) => setForm((f) => ({ ...f, [k]: v }))
  const save = useMutation({
    mutationFn: (patch: TemplateInput) => updateEmailTemplate(record.key, patch),
    onSuccess: async (saved) => {
      flash('notice', 'Template saved')
      qc.setQueryData<EmailTemplate[]>(['email', 'templates'], (list) => list?.map((t) => (t.key === saved.key ? saved : t)))
      await qc.invalidateQueries({ queryKey: ['email', 'templates'] })
    },
  })
  const { fields, banner } = splitErrors(save.error, KNOWN)

  function submit(e: FormEvent) {
    e.preventDefault()
    clear()
    const patch = changedFields(initial, form)
    if (Object.keys(patch).length === 0) { flash('info', 'No changes to save'); return }
    save.mutate(patch)
  }

  return (
    <form onSubmit={submit}>
      <StickyBar title={`Email Template: ${record.key}`} />
      {banner ? <Banner level="error">{errorMessage(banner)}</Banner> : null}
      <FormTable sections={[
        { title: 'Template', rows: [
          { id: 'tpl-subject', label: 'Subject', error: fields.subject,
            control: <input id="tpl-subject" type="text" className={s.body} value={form.subject} onChange={(e) => set('subject', e.target.value)} /> },
          { id: 'tpl-html', label: 'HTML body', error: fields.body_html,
            control: <textarea id="tpl-html" rows={14} className={s.body} style={MONO} value={form.body_html} onChange={(e) => set('body_html', e.target.value)} /> },
          { id: 'tpl-text', label: 'Text body', error: fields.body_text,
            control: <textarea id="tpl-text" rows={10} className={s.body} value={form.body_text} onChange={(e) => set('body_text', e.target.value)} /> },
          { label: 'Variables',
            control: (
              <ul className={s.vars} aria-label="Template variables">
                {VARIABLES.map(([v, meaning]) => <li key={v}><code style={MONO}>{v}</code><span className={s.varHelp}>{meaning}</span></li>)}
              </ul>
            ) },
        ] },
      ]} />
      <FormActions saving={save.isPending} onReset={() => setForm(initial)} cancelTo={LIST} />
    </form>
  )
}

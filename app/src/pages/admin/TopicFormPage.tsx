import { useMemo, useState, type FormEvent } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import type { Department, Priority, Topic, TopicInput } from '../../api/types'
import { LoadingScreen } from '../../components/LoadingScreen'
import { useTopicMutations } from '../../hooks/useAdminMutations'
import { useLastSeen } from '../../hooks/useLastSeen'
import { useReferenceData } from '../../hooks/useReferenceData'
import { changedFields, parseId, splitErrors } from '../../lib/forms'
import { useSubNav } from '../../ui/AppShell'
import { Banner, errorMessage } from '../../ui/Banner'
import { useBanner } from '../../ui/BannerContext'
import { FormActions } from '../../ui/FormActions'
import { FormTable } from '../../ui/FormTable'
import { StickyBar } from '../../ui/StickyBar'
import { NotFoundPage } from '../NotFoundPage'
import { adminSubNav } from './adminNav'
import { RefDataError } from './RefDataError'

const KNOWN = ['name', 'dept_id', 'priority_id', 'is_active', 'sort_order']
const LIST = '/admin/topics'
const EMPTY: TopicInput = { name: '', dept_id: null, priority_id: null, is_active: true, sort_order: 0 }
const toInput = (t: Topic): TopicInput => ({ name: t.name, dept_id: t.dept_id, priority_id: t.priority_id, is_active: t.is_active, sort_order: t.sort_order })

export function TopicFormPage() {
  const { id } = useParams()
  const { topics, departments, priorities, isLoading, error } = useReferenceData()
  const nav = useMemo(() => adminSubNav('topics'), [])
  useSubNav(nav.items, nav.right)
  const n = id === undefined ? null : parseId(id)
  const record = useLastSeen(n === null ? undefined : topics.find((t) => t.id === n), n)
  if (isLoading) return <LoadingScreen />
  if (error) return <RefDataError error={error} />
  if (id !== undefined && !record) return <NotFoundPage />
  return <TopicForm key={record?.id ?? 'new'} record={record} departments={departments} priorities={priorities} />
}

function TopicForm({ record, departments, priorities }: { record?: Topic; departments: Department[]; priorities: Priority[] }) {
  const navigate = useNavigate()
  const { flash, clear } = useBanner()
  const { create, update } = useTopicMutations()
  const initial = record ? toInput(record) : EMPTY
  const [form, setForm] = useState<TopicInput>(initial)
  // Kept as typed so '' and a lone '-' survive while editing; parsed on submit.
  const [sortText, setSortText] = useState(() => String(initial.sort_order))
  const set = <K extends keyof TopicInput>(k: K, v: TopicInput[K]) => setForm((f) => ({ ...f, [k]: v }))
  const active = record ? update : create
  const { fields, banner } = splitErrors(active.error, KNOWN)
  const optional = (v: string) => (v ? Number(v) : null)
  const saved = () => flash('notice', 'Help topic saved')

  function reset() {
    setForm(initial)
    setSortText(String(initial.sort_order))
  }

  function submit(e: FormEvent) {
    e.preventDefault()
    clear()
    const parsed = Number.parseInt(sortText, 10)
    const value: TopicInput = { ...form, sort_order: Number.isNaN(parsed) ? 0 : parsed }
    if (record) {
      const patch = changedFields(toInput(record), value)
      if (Object.keys(patch).length === 0) { flash('info', 'No changes to save'); return }
      update.mutate({ id: record.id, input: patch }, { onSuccess: saved })
    } else {
      create.mutate(value, { onSuccess: () => { saved(); navigate(LIST) } })
    }
  }

  return (
    <form onSubmit={submit}>
      <StickyBar title={record ? `Help Topic: ${record.name}` : 'Add New Help Topic'} />
      {banner ? <Banner level="error">{errorMessage(banner)}</Banner> : null}
      <FormTable sections={[
        { title: 'Settings', rows: [
          { id: 'topic-name', label: 'Name', required: true, error: fields.name,
            control: <input id="topic-name" value={form.name} maxLength={128} aria-required="true" onChange={(e) => set('name', e.target.value)} /> },
          { id: 'topic-active', label: 'Active', error: fields.is_active, help: 'Inactive topics are hidden from new tickets',
            control: <input id="topic-active" type="checkbox" checked={form.is_active} onChange={(e) => set('is_active', e.target.checked)} /> },
          { id: 'topic-sort', label: 'Sort order', error: fields.sort_order,
            control: <input id="topic-sort" type="number" step={1} value={sortText} onChange={(e) => setSortText(e.target.value)} /> },
        ] },
        { title: 'Routing', rows: [
          { id: 'topic-dept', label: 'Department', error: fields.dept_id,
            control: (
              <select id="topic-dept" value={form.dept_id ?? ''} onChange={(e) => set('dept_id', optional(e.target.value))}>
                <option value="">— none —</option>
                {departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
              </select>
            ) },
          { id: 'topic-priority', label: 'Priority', error: fields.priority_id,
            control: (
              <select id="topic-priority" value={form.priority_id ?? ''} onChange={(e) => set('priority_id', optional(e.target.value))}>
                <option value="">— none —</option>
                {priorities.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
              </select>
            ) },
        ] },
      ]} />
      <FormActions saving={create.isPending || update.isPending}onReset={reset} cancelTo={LIST} saveLabel={record ? 'Save Changes' : 'Create'} />
    </form>
  )
}

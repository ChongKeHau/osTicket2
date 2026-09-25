import { useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import type { Department, Priority, Topic, TopicInput } from '../../api/types'
import { ErrorBanner } from '../../components/ErrorBanner'
import { CheckboxField, FormField } from '../../components/FormField'
import { LoadingScreen } from '../../components/LoadingScreen'
import { useTopicMutations } from '../../hooks/useAdminMutations'
import { useLastSeen } from '../../hooks/useLastSeen'
import { useReferenceData } from '../../hooks/useReferenceData'
import { changedFields, parseId, splitErrors } from '../../lib/forms'
import { NotFoundPage } from '../NotFoundPage'
import styles from './admin.module.css'

const KNOWN = ['name', 'dept_id', 'priority_id', 'is_active', 'sort_order']
const LIST = '/admin/topics'
const toInput = (t: Topic): TopicInput => ({ name: t.name, dept_id: t.dept_id, priority_id: t.priority_id, is_active: t.is_active, sort_order: t.sort_order })

export function TopicFormPage() {
  const qc = useQueryClient()
  const { id } = useParams()
  const { topics, departments, priorities, isLoading, error } = useReferenceData()
  const n = id === undefined ? null : parseId(id)
  const record = useLastSeen(n === null ? undefined : topics.find((t) => t.id === n), n)
  if (isLoading) return <LoadingScreen />
  if (error) return <ErrorBanner error={error} onRetry={() => void qc.refetchQueries({ queryKey: ['ref'] })} />
  if (id !== undefined && !record) return <NotFoundPage />
  return <TopicForm key={record?.id ?? 'new'} record={record} departments={departments} priorities={priorities} />
}

function TopicForm({ record, departments, priorities }: { record?: Topic; departments: Department[]; priorities: Priority[] }) {
  const navigate = useNavigate()
  const { create, update } = useTopicMutations()
  const [form, setForm] = useState<TopicInput>(() => (record ? toInput(record) : { name: '', dept_id: null, priority_id: null, is_active: true, sort_order: 0 }))
  // Kept as typed so '' and a lone '-' survive while editing; parsed on submit.
  const [sortText, setSortText] = useState(() => String(form.sort_order))
  const set = <K extends keyof TopicInput>(k: K, v: TopicInput[K]) => setForm((f) => ({ ...f, [k]: v }))
  const active = record ? update : create
  const { fields, banner } = splitErrors(active.error, KNOWN)
  const done = () => navigate(LIST)
  const optional = (v: string) => (v ? Number(v) : null)

  function submit(e: FormEvent) {
    e.preventDefault()
    const parsed = Number.parseInt(sortText, 10)
    const value: TopicInput = { ...form, sort_order: Number.isNaN(parsed) ? 0 : parsed }
    if (record) {
      const patch = changedFields(toInput(record), value)
      if (Object.keys(patch).length === 0) { done(); return }
      update.mutate({ id: record.id, input: patch }, { onSuccess: done })
    } else {
      create.mutate(value, { onSuccess: done })
    }
  }

  return (
    <form onSubmit={submit} className={`panel ${styles.form}`}>
      <h1 style={{ marginTop: 0 }}>{record ? `Edit ${record.name}` : 'New topic'}</h1>
      {banner ? <ErrorBanner error={banner} /> : null}
      <FormField label="Name" error={fields.name}>
        <input value={form.name} maxLength={128} onChange={(e) => set('name', e.target.value)} />
      </FormField>
      <FormField label="Default department" error={fields.dept_id}>
        <select value={form.dept_id ?? ''} onChange={(e) => set('dept_id', optional(e.target.value))}>
          <option value="">None</option>
          {departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
        </select>
      </FormField>
      <FormField label="Default priority" error={fields.priority_id}>
        <select value={form.priority_id ?? ''} onChange={(e) => set('priority_id', optional(e.target.value))}>
          <option value="">None</option>
          {priorities.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
        </select>
      </FormField>
      <CheckboxField label="Active" checked={form.is_active} onChange={(v) => set('is_active', v)} />
      <FormField label="Sort order" error={fields.sort_order}>
        <input type="number" step={1} value={sortText} onChange={(e) => setSortText(e.target.value)} />
      </FormField>
      <div className="row">
        <button type="submit" className="primary" disabled={active.isPending}>{record ? 'Save' : 'Create'}</button>
        <Link to={LIST}>Cancel</Link>
      </div>
    </form>
  )
}

import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import type { Department, Priority, Topic, TopicInput } from '../../api/types'
import { ErrorBanner } from '../../components/ErrorBanner'
import { CheckboxField, FormField } from '../../components/FormField'
import { LoadingScreen } from '../../components/LoadingScreen'
import { useTopicMutations } from '../../hooks/useAdminMutations'
import { useReferenceData } from '../../hooks/useReferenceData'
import { changedFields, parseId, splitErrors } from '../../lib/forms'
import { NotFoundPage } from '../NotFoundPage'
import styles from './admin.module.css'

const KNOWN = ['name', 'dept_id', 'priority_id', 'is_active', 'sort_order']
const LIST = '/admin/topics'
const toInput = (t: Topic): TopicInput => ({ name: t.name, dept_id: t.dept_id, priority_id: t.priority_id, is_active: t.is_active, sort_order: t.sort_order })

export function TopicFormPage() {
  const { id } = useParams()
  const { topics, departments, priorities, isLoading, error } = useReferenceData()
  if (isLoading) return <LoadingScreen />
  if (error) return <ErrorBanner error={error} />
  let record: Topic | undefined
  if (id !== undefined) {
    const n = parseId(id)
    record = n === null ? undefined : topics.find((t) => t.id === n)
    if (!record) return <NotFoundPage />
  }
  return <TopicForm key={record?.id ?? 'new'} record={record} departments={departments} priorities={priorities} />
}

function TopicForm({ record, departments, priorities }: { record?: Topic; departments: Department[]; priorities: Priority[] }) {
  const navigate = useNavigate()
  const { create, update } = useTopicMutations()
  const [form, setForm] = useState<TopicInput>(() => (record ? toInput(record) : { name: '', dept_id: null, priority_id: null, is_active: true, sort_order: 0 }))
  const set = <K extends keyof TopicInput>(k: K, v: TopicInput[K]) => setForm((f) => ({ ...f, [k]: v }))
  const active = record ? update : create
  const { fields, banner } = splitErrors(active.error, KNOWN)
  const done = () => navigate(LIST)
  const optional = (v: string) => (v ? Number(v) : null)

  function submit(e: FormEvent) {
    e.preventDefault()
    if (record) {
      const patch = changedFields(toInput(record), form)
      if (Object.keys(patch).length === 0) { done(); return }
      update.mutate({ id: record.id, input: patch }, { onSuccess: done })
    } else {
      create.mutate(form, { onSuccess: done })
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
        <input type="number" value={form.sort_order} onChange={(e) => set('sort_order', Number(e.target.value) || 0)} />
      </FormField>
      <div className="row">
        <button type="submit" className="primary" disabled={active.isPending}>{record ? 'Save' : 'Create'}</button>
        <Link to={LIST}>Cancel</Link>
      </div>
    </form>
  )
}

import { useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import type { Department, DepartmentInput, Staff } from '../../api/types'
import { ErrorBanner } from '../../components/ErrorBanner'
import { CheckboxField, FormField } from '../../components/FormField'
import { LoadingScreen } from '../../components/LoadingScreen'
import { useDepartmentMutations } from '../../hooks/useAdminMutations'
import { useLastSeen } from '../../hooks/useLastSeen'
import { useReferenceData } from '../../hooks/useReferenceData'
import { staffName } from '../../lib/format'
import { changedFields, parseId, splitErrors } from '../../lib/forms'
import { NotFoundPage } from '../NotFoundPage'
import styles from './admin.module.css'

const KNOWN = ['name', 'is_public', 'manager_id']
const LIST = '/admin/departments'
const toInput = (d: Department): DepartmentInput => ({ name: d.name, is_public: d.is_public, manager_id: d.manager_id })

export function DepartmentFormPage() {
  const qc = useQueryClient()
  const { id } = useParams()
  const { departments, staff, isLoading, error } = useReferenceData()
  const n = id === undefined ? null : parseId(id)
  const record = useLastSeen(n === null ? undefined : departments.find((d) => d.id === n), n)
  if (isLoading) return <LoadingScreen />
  if (error) return <ErrorBanner error={error} onRetry={() => void qc.refetchQueries({ queryKey: ['ref'] })} />
  if (id !== undefined && !record) return <NotFoundPage />
  // key remounts the form (and reseeds its state) when navigating between records.
  return <DepartmentForm key={record?.id ?? 'new'} record={record} staff={staff} />
}

function DepartmentForm({ record, staff }: { record?: Department; staff: Staff[] }) {
  const navigate = useNavigate()
  const { create, update } = useDepartmentMutations()
  const [form, setForm] = useState<DepartmentInput>(() => (record ? toInput(record) : { name: '', is_public: true, manager_id: null }))
  const set = <K extends keyof DepartmentInput>(k: K, v: DepartmentInput[K]) => setForm((f) => ({ ...f, [k]: v }))
  const active = record ? update : create
  const { fields, banner } = splitErrors(active.error, KNOWN)
  const done = () => navigate(LIST)

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
      <h1 style={{ marginTop: 0 }}>{record ? `Edit ${record.name}` : 'New department'}</h1>
      {banner ? <ErrorBanner error={banner} /> : null}
      <FormField label="Name" error={fields.name}>
        <input value={form.name} maxLength={128} onChange={(e) => set('name', e.target.value)} />
      </FormField>
      <CheckboxField label="Public" checked={form.is_public} onChange={(v) => set('is_public', v)} />
      <FormField label="Manager" error={fields.manager_id}>
        <select value={form.manager_id ?? ''} onChange={(e) => set('manager_id', e.target.value ? Number(e.target.value) : null)}>
          <option value="">None</option>
          {staff.map((s) => <option key={s.id} value={s.id}>{staffName(s)}</option>)}
        </select>
      </FormField>
      <div className="row">
        <button type="submit" className="primary" disabled={active.isPending}>{record ? 'Save' : 'Create'}</button>
        <Link to={LIST}>Cancel</Link>
      </div>
    </form>
  )
}

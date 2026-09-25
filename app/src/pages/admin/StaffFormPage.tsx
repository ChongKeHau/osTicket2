import { useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import type { Department, Staff, UpdateStaffInput } from '../../api/types'
import { ErrorBanner } from '../../components/ErrorBanner'
import { CheckboxField, FormField } from '../../components/FormField'
import { LoadingScreen } from '../../components/LoadingScreen'
import { useStaffMutations } from '../../hooks/useAdminMutations'
import { useLastSeen } from '../../hooks/useLastSeen'
import { useReferenceData } from '../../hooks/useReferenceData'
import { staffName } from '../../lib/format'
import { changedFields, parseId, splitErrors } from '../../lib/forms'
import { NotFoundPage } from '../NotFoundPage'
import styles from './admin.module.css'

const LIST = '/admin/staff'
const EDIT_KNOWN = ['email', 'first_name', 'last_name', 'is_admin', 'is_active', 'primary_dept_id', 'department_ids']
// Create does not render is_active, so an error on it must reach the banner.
const CREATE_KNOWN = ['username', 'password', 'email', 'first_name', 'last_name', 'is_admin', 'primary_dept_id', 'department_ids']

/** Editable values; primary_dept_id 0 means "not chosen yet" and is sent as 0 so the API reports it. */
interface Form {
  username: string; email: string; password: string; first_name: string; last_name: string
  is_admin: boolean; is_active: boolean; primary_dept_id: number; department_ids: number[]
}

const fromRecord = (s: Staff): Form => ({
  username: s.username, email: s.email, password: '', first_name: s.first_name, last_name: s.last_name,
  is_admin: s.is_admin, is_active: s.is_active, primary_dept_id: s.primary_dept_id, department_ids: s.department_ids,
})
const EMPTY: Form = { username: '', email: '', password: '', first_name: '', last_name: '', is_admin: false, is_active: true, primary_dept_id: 0, department_ids: [] }

/** The primary department is always a membership. */
const withPrimary = (ids: number[], primary: number) => (primary && !ids.includes(primary) ? [...ids, primary] : ids)

const toUpdate = (f: Form): Required<UpdateStaffInput> => ({
  email: f.email, first_name: f.first_name, last_name: f.last_name, is_admin: f.is_admin, is_active: f.is_active,
  primary_dept_id: f.primary_dept_id, department_ids: withPrimary(f.department_ids, f.primary_dept_id),
})

export function StaffFormPage() {
  const qc = useQueryClient()
  const { id } = useParams()
  const { staff, departments, isLoading, error } = useReferenceData()
  const n = id === undefined ? null : parseId(id)
  const record = useLastSeen(n === null ? undefined : staff.find((s) => s.id === n), n)
  if (isLoading) return <LoadingScreen />
  if (error) return <ErrorBanner error={error} onRetry={() => void qc.refetchQueries({ queryKey: ['ref'] })} />
  if (id !== undefined && !record) return <NotFoundPage />
  return <StaffForm key={record?.id ?? 'new'} record={record} departments={departments} />
}

function StaffForm({ record, departments }: { record?: Staff; departments: Department[] }) {
  const navigate = useNavigate()
  const { create, update } = useStaffMutations()
  const [form, setForm] = useState<Form>(() => (record ? fromRecord(record) : EMPTY))
  const set = <K extends keyof Form>(k: K, v: Form[K]) => setForm((f) => ({ ...f, [k]: v }))
  const toggleDept = (id: number, on: boolean) => set('department_ids', on ? [...form.department_ids, id] : form.department_ids.filter((d) => d !== id))
  const active = record ? update : create
  const { fields, banner } = splitErrors(active.error, record ? EDIT_KNOWN : CREATE_KNOWN)
  const done = () => navigate(LIST)

  function submit(e: FormEvent) {
    e.preventDefault()
    if (record) {
      const patch = changedFields(toUpdate(fromRecord(record)), toUpdate(form))
      if (Object.keys(patch).length === 0) { done(); return }
      update.mutate({ id: record.id, input: patch }, { onSuccess: done })
    } else {
      create.mutate({
        username: form.username, email: form.email, password: form.password, first_name: form.first_name, last_name: form.last_name,
        is_admin: form.is_admin, primary_dept_id: form.primary_dept_id, department_ids: withPrimary(form.department_ids, form.primary_dept_id),
      }, { onSuccess: done })
    }
  }

  return (
    <>
      <form onSubmit={submit} className={`panel ${styles.form}`}>
        <h1 style={{ marginTop: 0 }}>{record ? `Edit ${staffName(record)}` : 'New staff member'}</h1>
        {banner ? <ErrorBanner error={banner} /> : null}
        {!record && (
          <FormField label="Username" error={fields.username}>
            <input value={form.username} maxLength={64} autoComplete="off" onChange={(e) => set('username', e.target.value)} />
          </FormField>
        )}
        <FormField label="Email" error={fields.email}>
          <input type="email" value={form.email} maxLength={255} onChange={(e) => set('email', e.target.value)} />
        </FormField>
        {!record && (
          <FormField label="Password" error={fields.password}>
            <input type="password" value={form.password} autoComplete="new-password" onChange={(e) => set('password', e.target.value)} />
          </FormField>
        )}
        <FormField label="First name" error={fields.first_name}>
          <input value={form.first_name} maxLength={64} onChange={(e) => set('first_name', e.target.value)} />
        </FormField>
        <FormField label="Last name" error={fields.last_name}>
          <input value={form.last_name} maxLength={64} onChange={(e) => set('last_name', e.target.value)} />
        </FormField>
        <CheckboxField label="Administrator" checked={form.is_admin} onChange={(v) => set('is_admin', v)} />
        {record && <CheckboxField label="Active" checked={form.is_active} onChange={(v) => set('is_active', v)} />}
        <FormField label="Primary department" error={fields.primary_dept_id}>
          <select value={form.primary_dept_id || ''} onChange={(e) => set('primary_dept_id', Number(e.target.value) || 0)}>
            <option value="">Choose…</option>
            {departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
          </select>
        </FormField>
        <fieldset style={{ border: 'none', padding: 0, margin: 0 }}>
          <legend className="muted" style={{ marginBottom: 6 }}>Departments</legend>
          <div className={styles.checks}>
            {departments.map((d) => (
              <label key={d.id} className="row" style={{ gap: 6 }}>
                <input type="checkbox" checked={form.department_ids.includes(d.id)} onChange={(e) => toggleDept(d.id, e.target.checked)} />
                <span>{d.name}</span>
              </label>
            ))}
          </div>
          {fields.department_ids && <span className="field-error">{fields.department_ids}</span>}
        </fieldset>
        <div className="row">
          <button type="submit" className="primary" disabled={active.isPending}>{record ? 'Save' : 'Create'}</button>
          <Link to={LIST}>Cancel</Link>
        </div>
      </form>
      {record && <SetPassword id={record.id} />}
    </>
  )
}

function SetPassword({ id }: { id: number }) {
  const { setPassword } = useStaffMutations()
  const [password, setValue] = useState('')
  const [updated, setUpdated] = useState(false)
  const { fields, banner } = splitErrors(setPassword.error, ['password'])

  function submit(e: FormEvent) {
    e.preventDefault()
    setUpdated(false)
    setPassword.mutate({ id, password }, { onSuccess: () => { setValue(''); setUpdated(true) } })
  }

  return (
    <form onSubmit={submit} className={`panel ${styles.form}`}>
      <h2 style={{ marginTop: 0 }}>Set password</h2>
      {banner ? <ErrorBanner error={banner} /> : null}
      {updated && <p role="status">Password updated</p>}
      <FormField label="New password" error={fields.password}>
        <input type="password" value={password} autoComplete="new-password" onChange={(e) => setValue(e.target.value)} />
      </FormField>
      <button type="submit" disabled={setPassword.isPending || password === ''}>Set password</button>
    </form>
  )
}

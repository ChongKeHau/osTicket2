import { useMemo, useState, type FormEvent } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import type { Department, Staff, UpdateStaffInput } from '../../api/types'
import { LoadingScreen } from '../../components/LoadingScreen'
import { useStaffMutations } from '../../hooks/useAdminMutations'
import { useLastSeen } from '../../hooks/useLastSeen'
import { useReferenceData } from '../../hooks/useReferenceData'
import { staffName } from '../../lib/format'
import { changedFields, parseId, splitErrors } from '../../lib/forms'
import { useSubNav } from '../../ui/AppShell'
import { Banner, errorMessage } from '../../ui/Banner'
import { useBanner } from '../../ui/BannerContext'
import { FormActions } from '../../ui/FormActions'
import { FormTable, type FormRow } from '../../ui/FormTable'
import { StickyBar } from '../../ui/StickyBar'
import { NotFoundPage } from '../NotFoundPage'
import { adminSubNav } from './adminNav'
import { RefDataError } from './RefDataError'

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
  const { id } = useParams()
  const { staff, departments, isLoading, error } = useReferenceData()
  const nav = useMemo(() => adminSubNav('staff'), [])
  useSubNav(nav.items, nav.right)
  const n = id === undefined ? null : parseId(id)
  const record = useLastSeen(n === null ? undefined : staff.find((s) => s.id === n), n)
  if (isLoading) return <LoadingScreen />
  if (error) return <RefDataError error={error} />
  if (id !== undefined && !record) return <NotFoundPage />
  return <StaffForm key={record?.id ?? 'new'} record={record} departments={departments} />
}

function StaffForm({ record, departments }: { record?: Staff; departments: Department[] }) {
  const navigate = useNavigate()
  const { flash, clear } = useBanner()
  const { create, update } = useStaffMutations()
  const initial = record ? fromRecord(record) : EMPTY
  const [form, setForm] = useState<Form>(initial)
  const set = <K extends keyof Form>(k: K, v: Form[K]) => setForm((f) => ({ ...f, [k]: v }))
  const active = record ? update : create
  const { fields, banner } = splitErrors(active.error, record ? EDIT_KNOWN : CREATE_KNOWN)
  const saved = () => flash('notice', 'Staff member saved')

  function submit(e: FormEvent) {
    e.preventDefault()
    clear()
    if (record) {
      const patch = changedFields(toUpdate(fromRecord(record)), toUpdate(form))
      if (Object.keys(patch).length === 0) { flash('info', 'No changes to save'); return }
      update.mutate({ id: record.id, input: patch }, { onSuccess: saved })
    } else {
      create.mutate({
        username: form.username, email: form.email, password: form.password, first_name: form.first_name, last_name: form.last_name,
        is_admin: form.is_admin, primary_dept_id: form.primary_dept_id, department_ids: withPrimary(form.department_ids, form.primary_dept_id),
      }, { onSuccess: () => { saved(); navigate(LIST) } })
    }
  }

  const account: FormRow[] = [
    record
      ? { label: 'Username', control: <span>{record.username}</span> }
      : { id: 'staff-username', label: 'Username', required: true, error: fields.username,
          control: <input id="staff-username" value={form.username} maxLength={64} autoComplete="off" aria-required="true" onChange={(e) => set('username', e.target.value)} /> },
    { id: 'staff-email', label: 'Email', required: true, error: fields.email,
      control: <input id="staff-email" type="email" value={form.email} maxLength={255} aria-required="true" onChange={(e) => set('email', e.target.value)} /> },
    { id: 'staff-first', label: 'First name', error: fields.first_name,
      control: <input id="staff-first" value={form.first_name} maxLength={64} onChange={(e) => set('first_name', e.target.value)} /> },
    { id: 'staff-last', label: 'Last name', error: fields.last_name,
      control: <input id="staff-last" value={form.last_name} maxLength={64} onChange={(e) => set('last_name', e.target.value)} /> },
  ]
  if (!record) {
    account.push({ id: 'staff-password', label: 'Password', required: true, error: fields.password,
      control: <input id="staff-password" type="password" value={form.password} autoComplete="new-password" aria-required="true" onChange={(e) => set('password', e.target.value)} /> })
  }

  const permissions: FormRow[] = [
    { id: 'staff-admin', label: 'Administrator', error: fields.is_admin,
      control: <input id="staff-admin" type="checkbox" checked={form.is_admin} onChange={(e) => set('is_admin', e.target.checked)} /> },
  ]
  if (record) {
    permissions.push({ id: 'staff-active', label: 'Active', error: fields.is_active,
      control: <input id="staff-active" type="checkbox" checked={form.is_active} onChange={(e) => set('is_active', e.target.checked)} /> })
  }

  const memberships: FormRow[] = [
    { id: 'staff-primary', label: 'Primary department', required: true, error: fields.primary_dept_id,
      control: (
        <select id="staff-primary" value={form.primary_dept_id || ''} aria-required="true" onChange={(e) => set('primary_dept_id', Number(e.target.value) || 0)}>
          <option value="">— select —</option>
          {departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
        </select>
      ) },
    { id: 'staff-depts', label: 'Additional departments', error: fields.department_ids, help: 'The primary department is always included',
      control: (
        <select id="staff-depts" multiple size={Math.min(Math.max(departments.length, 2), 6)} value={form.department_ids.map(String)}
          onChange={(e) => set('department_ids', Array.from(e.target.selectedOptions, (o) => Number(o.value)))}>
          {departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
        </select>
      ) },
  ]

  return (
    <>
      <form onSubmit={submit}>
        <StickyBar title={record ? `Staff Member: ${staffName(record)}` : 'Add New Staff Member'} />
        {banner ? <Banner level="error">{errorMessage(banner)}</Banner> : null}
        <FormTable sections={[
          { title: 'Account', rows: account },
          { title: 'Permissions', rows: permissions },
          { title: 'Departments', rows: memberships },
        ]} />
        <FormActions saving={create.isPending || update.isPending} onReset={() => setForm(initial)} cancelTo={LIST} saveLabel={record ? 'Save Changes' : 'Create'} />
      </form>
      {record && <ChangePassword id={record.id} />}
    </>
  )
}

function ChangePassword({ id }: { id: number }) {
  const { flash, clear } = useBanner()
  const { setPassword } = useStaffMutations()
  const [password, setValue] = useState('')
  const { fields, banner } = splitErrors(setPassword.error, ['password'])

  function submit(e: FormEvent) {
    e.preventDefault()
    clear()
    setPassword.mutate({ id, password }, { onSuccess: () => { setValue(''); flash('notice', 'Password updated') } })
  }

  return (
    <form onSubmit={submit}>
      {banner ? <Banner level="error">{errorMessage(banner)}</Banner> : null}
      <FormTable sections={[
        { title: 'Change Password', rows: [
          { id: 'staff-new-password', label: 'Password', required: true, error: fields.password,
            control: <input id="staff-new-password" type="password" value={password} autoComplete="new-password" aria-required="true" onChange={(e) => setValue(e.target.value)} /> },
        ] },
      ]} />
      <FormActions saving={setPassword.isPending} cancelTo={LIST} saveLabel="Change Password" />
    </form>
  )
}

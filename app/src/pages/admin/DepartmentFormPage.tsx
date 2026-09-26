import { useState, type FormEvent } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import type { Department, DepartmentInput, Staff } from '../../api/types'
import { LoadingScreen } from '../../components/LoadingScreen'
import { useDepartmentMutations } from '../../hooks/useAdminMutations'
import { useLastSeen } from '../../hooks/useLastSeen'
import { useReferenceData } from '../../hooks/useReferenceData'
import { staffName } from '../../lib/format'
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

const KNOWN = ['name', 'is_public', 'manager_id']
const LIST = '/admin/departments'
const EMPTY: DepartmentInput = { name: '', is_public: true, manager_id: null }
const toInput = (d: Department): DepartmentInput => ({ name: d.name, is_public: d.is_public, manager_id: d.manager_id })

export function DepartmentFormPage() {
  const { id } = useParams()
  const { departments, staff, isLoading, error } = useReferenceData()
  useSubNav(adminSubNav('departments'))
  const n = id === undefined ? null : parseId(id)
  const record = useLastSeen(n === null ? undefined : departments.find((d) => d.id === n), n)
  if (isLoading) return <LoadingScreen />
  if (error) return <RefDataError error={error} />
  if (id !== undefined && !record) return <NotFoundPage />
  // key remounts the form (and reseeds its state) when navigating between records.
  return <DepartmentForm key={record?.id ?? 'new'} record={record} staff={staff} />
}

function DepartmentForm({ record, staff }: { record?: Department; staff: Staff[] }) {
  const navigate = useNavigate()
  const { flash, clear } = useBanner()
  const { create, update } = useDepartmentMutations()
  const initial = record ? toInput(record) : EMPTY
  const [form, setForm] = useState<DepartmentInput>(initial)
  const set = <K extends keyof DepartmentInput>(k: K, v: DepartmentInput[K]) => setForm((f) => ({ ...f, [k]: v }))
  const active = record ? update : create
  const { fields, banner } = splitErrors(active.error, KNOWN)
  const saved = () => flash('notice', 'Department saved')
  // Active staff, plus the current manager even if since deactivated so the select can show it.
  const managers = staff.filter((s) => s.is_active || s.id === form.manager_id)

  function submit(e: FormEvent) {
    e.preventDefault()
    clear()
    if (record) {
      const patch = changedFields(toInput(record), form)
      if (Object.keys(patch).length === 0) { flash('info', 'No changes to save'); return }
      update.mutate({ id: record.id, input: patch }, { onSuccess: saved })
    } else {
      create.mutate(form, { onSuccess: () => { saved(); navigate(LIST) } })
    }
  }

  return (
    <form onSubmit={submit}>
      <StickyBar title={record ? `Department: ${record.name}` : 'Add New Department'} />
      {banner ? <Banner level="error">{errorMessage(banner)}</Banner> : null}
      <FormTable sections={[
        { title: 'Settings', rows: [
          { id: 'dept-name', label: 'Name', required: true, error: fields.name,
            control: <input id="dept-name" value={form.name} maxLength={128} aria-required="true" onChange={(e) => set('name', e.target.value)} /> },
          { id: 'dept-public', label: 'Public', error: fields.is_public, help: 'Public departments are visible to end users',
            control: <input id="dept-public" type="checkbox" checked={form.is_public} onChange={(e) => set('is_public', e.target.checked)} /> },
          { id: 'dept-manager', label: 'Manager', error: fields.manager_id,
            control: (
              <select id="dept-manager" value={form.manager_id ?? ''} onChange={(e) => set('manager_id', e.target.value ? Number(e.target.value) : null)}>
                <option value="">— none —</option>
                {managers.map((s) => <option key={s.id} value={s.id}>{staffName(s)}</option>)}
              </select>
            ) },
        ] },
      ]} />
      <FormActions saving={create.isPending || update.isPending} onReset={() => setForm(initial)} cancelTo={LIST} saveLabel={record ? 'Save Changes' : 'Create'} />
    </form>
  )
}

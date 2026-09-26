import { useMemo, useState } from 'react'
import type { Staff } from '../../api/types'
import { useReferenceData } from '../../hooks/useReferenceData'
import { staffName } from '../../lib/format'
import { sortBy } from '../../lib/sort'
import { useSubNav } from '../../ui/AppShell'
import { LinkButton } from '../../ui/Button'
import { ListTable, type Column } from '../../ui/ListTable'
import { StickyBar } from '../../ui/StickyBar'
import { adminSubNav } from './adminNav'
import { RefDataError } from './RefDataError'

export function StaffListPage() {
  const { staff, departments, isLoading, error } = useReferenceData()
  const nav = useMemo(() => adminSubNav('staff'), [])
  useSubNav(nav.items, nav.right)
  const [sort, setSort] = useState('username')
  const rows = useMemo(() => sortBy(staff, sort), [staff, sort])
  const dept = (id: number) => departments.find((d) => d.id === id)?.name ?? '—'

  // Staff are deactivated, never deleted, so there is no actions column.
  const columns: Column<Staff>[] = [
    { key: 'name', label: 'Name', sortKey: 'username', render: (s) => staffName(s) },
    { key: 'username', label: 'Username', render: (s) => s.username },
    { key: 'email', label: 'Email', render: (s) => s.email },
    { key: 'role', label: 'Role', sortKey: 'is_admin', render: (s) => (s.is_admin ? 'Administrator' : 'Agent') },
    { key: 'status', label: 'Status', sortKey: 'is_active', render: (s) => (s.is_active ? 'Active' : 'Inactive') },
    { key: 'dept', label: 'Department', render: (s) => dept(s.primary_dept_id) },
  ]

  return (
    <>
      <StickyBar title="Staff" count={staff.length} actions={<LinkButton to="/admin/staff/new" variant="add">Add New Staff Member</LinkButton>} />
      {error && <RefDataError error={error} />}
      <ListTable columns={columns} rows={rows} rowKey={(s) => s.id} sort={sort} onSort={setSort}
        rowHref={(s) => `/admin/staff/${s.id}`} empty={isLoading ? 'Loading…' : 'No staff'}
        footer={<span>{staff.length} staff members</span>} />
    </>
  )
}

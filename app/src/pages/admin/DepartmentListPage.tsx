import { useMemo, useState } from 'react'
import type { Department } from '../../api/types'
import { ConfirmDelete } from '../../components/ConfirmDelete'
import { useDepartmentMutations } from '../../hooks/useAdminMutations'
import { useReferenceData } from '../../hooks/useReferenceData'
import { staffName } from '../../lib/format'
import { sortBy } from '../../lib/sort'
import { useSubNav } from '../../ui/AppShell'
import { Banner, errorMessage } from '../../ui/Banner'
import { useBanner } from '../../ui/BannerContext'
import { LinkButton } from '../../ui/Button'
import { ListTable, type Column } from '../../ui/ListTable'
import { Menu } from '../../ui/Menu'
import { StickyBar } from '../../ui/StickyBar'
import { adminSubNav } from './adminNav'
import { RefDataError } from './RefDataError'

export function DepartmentListPage() {
  const { departments, staff, isLoading, error } = useReferenceData()
  const { remove } = useDepartmentMutations()
  const { flash } = useBanner()
  const nav = useMemo(() => adminSubNav('departments'), [])
  useSubNav(nav.items, nav.right)
  const [sort, setSort] = useState('name')
  const rows = useMemo(() => sortBy(departments, sort), [departments, sort])
  const [confirming, setConfirming] = useState<number | null>(null)
  const managerName = (id: number | null) => {
    const m = staff.find((s) => s.id === id)
    return m ? staffName(m) : '—'
  }

  const columns: Column<Department>[] = [
    { key: 'name', label: 'Name', sortKey: 'name', render: (d) => d.name },
    { key: 'public', label: 'Type', sortKey: 'is_public', render: (d) => (d.is_public ? 'Public' : 'Private') },
    { key: 'manager', label: 'Manager', render: (d) => managerName(d.manager_id) },
    {
      key: 'actions', label: '', width: '10em', align: 'right',
      render: (d) => (confirming === d.id
        ? <ConfirmDelete label={d.name} startConfirming disabled={remove.isPending} onCancel={() => setConfirming(null)}
            onConfirm={() => remove.mutateAsync(d.id).then(() => flash('notice', `Department "${d.name}" deleted`)).finally(() => setConfirming(null))} />
        : <Menu label="More" items={[{ label: 'Delete', danger: true, disabled: remove.isPending, onSelect: () => setConfirming(d.id) }]} />),
    },
  ]

  return (
    <>
      <StickyBar title="Departments" count={departments.length} actions={<LinkButton to="/admin/departments/new" variant="add">Add New Department</LinkButton>} />
      {error && <RefDataError error={error} />}
      {remove.error && <Banner level="error">{errorMessage(remove.error)}</Banner>}
      <ListTable columns={columns} rows={rows} rowKey={(d) => d.id} sort={sort} onSort={setSort}
        rowHref={(d) => `/admin/departments/${d.id}`} empty={isLoading ? 'Loading…' : 'No departments'}
        footer={<span>{departments.length} departments</span>} />
    </>
  )
}

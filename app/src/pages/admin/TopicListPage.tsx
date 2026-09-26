import { useMemo, useState } from 'react'
import type { Topic } from '../../api/types'
import { ConfirmDelete } from '../../components/ConfirmDelete'
import { useTopicMutations } from '../../hooks/useAdminMutations'
import { useReferenceData } from '../../hooks/useReferenceData'
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

const nameOf = (list: { id: number; name: string }[], id: number | null) => list.find((x) => x.id === id)?.name ?? '—'

export function TopicListPage() {
  const { topics, departments, priorities, isLoading, error } = useReferenceData()
  const { remove } = useTopicMutations()
  const { flash } = useBanner()
  const nav = useMemo(() => adminSubNav('topics'), [])
  useSubNav(nav.items, nav.right)
  const [sort, setSort] = useState('sort_order')
  const rows = useMemo(() => sortBy(topics, sort), [topics, sort])
  const [confirming, setConfirming] = useState<number | null>(null)

  const columns: Column<Topic>[] = [
    { key: 'name', label: 'Name', sortKey: 'name', render: (t) => t.name },
    { key: 'dept', label: 'Department', render: (t) => nameOf(departments, t.dept_id) },
    { key: 'priority', label: 'Priority', render: (t) => nameOf(priorities, t.priority_id) },
    { key: 'status', label: 'Status', sortKey: 'is_active', render: (t) => (t.is_active ? 'Active' : 'Disabled') },
    { key: 'sort', label: 'Sort order', sortKey: 'sort_order', align: 'right', render: (t) => String(t.sort_order) },
    {
      key: 'actions', label: '', width: '10em', align: 'right',
      render: (t) => (confirming === t.id
        ? <ConfirmDelete label={t.name} startConfirming disabled={remove.isPending} onCancel={() => setConfirming(null)}
            onConfirm={() => remove.mutateAsync(t.id).then(() => flash('notice', `Help topic "${t.name}" deleted`)).finally(() => setConfirming(null))} />
        : <Menu label="More" items={[{ label: 'Delete', danger: true, disabled: remove.isPending, onSelect: () => setConfirming(t.id) }]} />),
    },
  ]

  return (
    <>
      <StickyBar title="Help Topics" count={topics.length} actions={<LinkButton to="/admin/topics/new" variant="add">Add New Help Topic</LinkButton>} />
      {error && <RefDataError error={error} />}
      {remove.error && <Banner level="error">{errorMessage(remove.error)}</Banner>}
      <ListTable columns={columns} rows={rows} rowKey={(t) => t.id} sort={sort} onSort={setSort}
        rowHref={(t) => `/admin/topics/${t.id}`} empty={isLoading ? 'Loading…' : 'No help topics'}
        footer={<span>{topics.length} help topics</span>} />
    </>
  )
}

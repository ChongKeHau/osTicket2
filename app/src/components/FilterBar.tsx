import { useEffect, useState } from 'react'
import type { ListFilter } from '../api/types'
import { useDebouncedValue } from '../hooks/useDebouncedValue'
import { useReferenceData } from '../hooks/useReferenceData'
import { staffName } from '../lib/format'

export function FilterBar({ filter, onChange }: { filter: ListFilter; onChange: (patch: Partial<ListFilter>) => void }) {
  const { statuses, departments, staff } = useReferenceData()
  const [q, setQ] = useState(filter.q ?? '')
  const debouncedQ = useDebouncedValue(q, 300)
  useEffect(() => { if ((filter.q ?? '') !== debouncedQ) onChange({ q: debouncedQ || undefined }) }, [debouncedQ]) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="row panel">
      <label>State <select aria-label="State" value={filter.state ?? ''} onChange={(e) => onChange({ state: (e.target.value || undefined) as ListFilter['state'] })}>
        <option value="">All</option><option value="open">Open</option><option value="resolved">Resolved</option><option value="closed">Closed</option>
      </select></label>
      <label>Status <select aria-label="Status" value={filter.status ?? ''} onChange={(e) => onChange({ status: e.target.value ? Number(e.target.value) : undefined })}>
        <option value="">Any</option>{statuses.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
      </select></label>
      <label>Department <select aria-label="Department" value={filter.dept_id ?? ''} onChange={(e) => onChange({ dept_id: e.target.value ? Number(e.target.value) : undefined })}>
        <option value="">Any</option>{departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
      </select></label>
      <label>Assigned <select aria-label="Assigned" value={filter.assigned_to ?? ''} onChange={(e) => onChange({ assigned_to: e.target.value || undefined })}>
        <option value="">All</option><option value="me">Me</option><option value="none">Unassigned</option>
        {staff.filter((s) => s.is_active).map((s) => <option key={s.id} value={s.id}>{staffName(s)}</option>)}
      </select></label>
      <label>Search <input aria-label="Search" placeholder="Subject" value={q} onChange={(e) => setQ(e.target.value)} /></label>
    </div>
  )
}

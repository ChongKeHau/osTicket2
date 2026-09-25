import { useCallback, useMemo } from 'react'
import { useSearchParams } from 'react-router-dom'
import type { ListFilter, TicketState } from '../api/types'

const STATES: TicketState[] = ['open', 'resolved', 'closed']
export const SORTS = ['created_at', '-created_at', 'last_message_at', '-last_message_at', 'priority', '-priority'] as const
export const DEFAULT_SORT = '-last_message_at'
const DEFAULT_PAGE_SIZE = 25

function posInt(v: string | null, max = Number.MAX_SAFE_INTEGER): number | undefined {
  if (!v || !/^\d+$/.test(v)) return undefined
  const n = Number(v)
  return n >= 1 && n <= max ? n : undefined
}

export function parseFilter(params: URLSearchParams): ListFilter {
  const state = params.get('state')
  const sort = params.get('sort')
  const assigned = params.get('assigned_to')
  const f: ListFilter = {
    page: posInt(params.get('page'), 1_000_000) ?? 1,
    page_size: posInt(params.get('page_size'), 100) ?? DEFAULT_PAGE_SIZE,
    sort: sort && (SORTS as readonly string[]).includes(sort) ? sort : DEFAULT_SORT,
  }
  if (state && (STATES as string[]).includes(state)) f.state = state as TicketState
  const status = posInt(params.get('status')); if (status) f.status = status
  const dept = posInt(params.get('dept_id')); if (dept) f.dept_id = dept
  if (assigned === 'me' || assigned === 'none') f.assigned_to = assigned
  else { const id = posInt(assigned); if (id) f.assigned_to = String(id) }
  const q = params.get('q')?.trim(); if (q) f.q = q
  return f
}

export function useTicketFilters() {
  const [params, setParams] = useSearchParams()
  const filter = useMemo(() => parseFilter(params), [params])
  const set = useCallback((patch: Partial<ListFilter>) => {
    setParams((prev) => {
      const next = new URLSearchParams(prev)
      const resetPage = Object.keys(patch).some((k) => k !== 'page')
      for (const [k, v] of Object.entries(patch)) {
        if (v === undefined || v === '' || v === null) next.delete(k)
        else next.set(k, String(v))
      }
      if (resetPage) next.delete('page')
      if (next.get('page') === '1') next.delete('page')
      return next
    })
  }, [setParams])
  return { filter, set }
}

import { useLocation, useSearchParams } from 'react-router-dom'
import { parseId } from '../../../lib/forms'
import { useSubNav } from '../../../ui/AppShell'
import type { SubNavItem } from '../../../ui/SubNav'

export const EMAIL_SUBNAV: SubNavItem[] = [
  { label: 'Templates', to: '/admin/email/templates' },
  { label: 'Outbox', to: '/admin/email/outbox' },
  { label: 'Inbound Log', to: '/admin/email/inbound' },
]

/** Populates the Email sub-nav. Items match by pathname prefix, so the outbox stays active
 *  with a status filter in the query and Templates stays active on a template's form. */
export function useEmailSubNav() {
  const { pathname } = useLocation()
  useSubNav(EMAIL_SUBNAV.map((item) => ({ ...item, active: pathname.startsWith(item.to) })))
}

export const PAGE_SIZE = 25

/** URL-backed list state for the outbox and inbound log: `page` (default 1) plus any filters.
 *  `update` sets or (with null / '') removes params, keeping the rest. */
export function useListParams() {
  const [params, setParams] = useSearchParams()
  const page = parseId(params.get('page') ?? undefined) ?? 1
  function update(next: Record<string, string | null>) {
    setParams((prev) => {
      const out = new URLSearchParams(prev)
      for (const [k, v] of Object.entries(next)) {
        if (v) out.set(k, v)
        else out.delete(k)
      }
      return out
    })
  }
  return { params, page, update }
}

/** `Name <address>`, or the bare address when there is no name. */
export const addressee = (name: string, address: string) => (name ? `${name} <${address}>` : address)

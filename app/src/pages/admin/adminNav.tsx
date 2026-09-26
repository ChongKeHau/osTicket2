import type { ReactNode } from 'react'
import { LinkButton } from '../../ui/Button'
import type { SubNavItem } from '../../ui/SubNav'

const LABEL = {
  departments: ['Department', 'Departments'],
  topics: ['Help Topic', 'Help Topics'],
  staff: ['Staff Member', 'Staff'],
} as const

export type AdminKind = keyof typeof LABEL

/** Sub-nav for an admin list tab: the list itself, and "Add New …" on the right.
 *  Returns a fresh element each call, so callers memoise it before `useSubNav`. */
export function adminSubNav(kind: AdminKind): { items: SubNavItem[]; right: ReactNode } {
  const [one, many] = LABEL[kind]
  return {
    items: [{ label: `All ${many}`, to: `/admin/${kind}` }],
    right: <LinkButton to={`/admin/${kind}/new`} variant="add">Add New {one}</LinkButton>,
  }
}

import type { SubNavItem } from '../../ui/SubNav'

const PLURAL = { departments: 'Departments', topics: 'Help Topics', staff: 'Staff' } as const

export type AdminKind = keyof typeof PLURAL

/** Sub-nav items for an admin list tab. The right slot stays empty: each list's single
 *  "Add New …" control is the green button in its StickyBar. */
export function adminSubNav(kind: AdminKind): SubNavItem[] {
  return [{ label: `All ${PLURAL[kind]}`, to: `/admin/${kind}` }]
}

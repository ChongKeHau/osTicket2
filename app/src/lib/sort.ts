/** Comparable form of a cell value: numbers stay numbers, booleans become 0/1, null sorts as ''. */
function comparable(v: unknown): string | number {
  if (typeof v === 'number') return v
  if (typeof v === 'boolean') return v ? 1 : 0
  return v == null ? '' : String(v)
}

/**
 * In-memory sort for small unpaged lists. `sort` is a row key, optionally prefixed with `-`
 * for descending. Returns a new array; ties keep their input order.
 */
export function sortBy<T>(rows: T[], sort: string): T[] {
  const desc = sort.startsWith('-')
  const key = (desc ? sort.slice(1) : sort) as keyof T
  if (!key) return [...rows]
  const dir = desc ? -1 : 1
  return [...rows].sort((a, b) => {
    const x = comparable(a[key])
    const y = comparable(b[key])
    const cmp = typeof x === 'number' && typeof y === 'number'
      ? x - y
      : String(x).localeCompare(String(y), undefined, { numeric: true, sensitivity: 'base' })
    return cmp * dir
  })
}

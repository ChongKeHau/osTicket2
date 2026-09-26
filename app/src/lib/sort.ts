function sortValue(v: unknown): string | number {
  if (typeof v === 'boolean') return v ? 1 : 0
  if (typeof v === 'number') return v
  return String(v ?? '')
}

/** Client-side sort for small lists: `sort` is a row key, `-` prefixed for descending. */
export function sortBy<T>(rows: T[], sort: string): T[] {
  const desc = sort.startsWith('-')
  const key = (desc ? sort.slice(1) : sort) as keyof T
  const out = [...rows].sort((a, b) => {
    const x = sortValue(a[key]), y = sortValue(b[key])
    return typeof x === 'number' && typeof y === 'number' ? x - y : String(x).localeCompare(String(y), undefined, { numeric: true })
  })
  return desc ? out.reverse() : out
}

import { ApiError } from '../api/client'

/** Route params are strings; only a positive integer names a record. */
export function parseId(raw: string | undefined): number | null {
  if (raw === undefined || !/^[1-9]\d*$/.test(raw)) return null
  const n = Number(raw)
  return Number.isSafeInteger(n) ? n : null
}

function normalize(v: unknown): string {
  if (Array.isArray(v)) return JSON.stringify([...v].sort((a, b) => (a < b ? -1 : a > b ? 1 : 0)))
  return JSON.stringify(v ?? null)
}

/** Keys of `after` whose value differs from `before`; number arrays compare as sets. */
export function changedFields<T extends object>(before: T, after: T): Partial<T> {
  const out: Partial<T> = {}
  for (const key of Object.keys(after) as (keyof T)[]) {
    if (normalize(before[key]) !== normalize(after[key])) out[key] = after[key]
  }
  return out
}

/**
 * Field errors for fields the form renders go under the inputs; anything else (a conflict,
 * a network error, or an error on a field the form does not show) goes to the banner.
 */
export function splitErrors(error: unknown, known: string[]): { fields: Record<string, string>; banner: unknown } {
  if (!error) return { fields: {}, banner: null }
  if (!(error instanceof ApiError) || Object.keys(error.fields).length === 0) return { fields: {}, banner: error }
  const fields: Record<string, string> = {}
  let unknown = false
  for (const [k, v] of Object.entries(error.fields)) {
    if (known.includes(k)) fields[k] = v
    else unknown = true
  }
  return { fields, banner: unknown ? error : null }
}

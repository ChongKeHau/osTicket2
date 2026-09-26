import { sortBy } from './sort'

const rows = [
  { name: 'Sales', n: 10, on: true, opt: null as string | null },
  { name: 'billing', n: 2, on: false, opt: 'b' },
  { name: 'Support', n: 1, on: true, opt: 'a' },
]

it('sorts strings case-insensitively ascending, descending with a - prefix', () => {
  expect(sortBy(rows, 'name').map((r) => r.name)).toEqual(['billing', 'Sales', 'Support'])
  expect(sortBy(rows, '-name').map((r) => r.name)).toEqual(['Support', 'Sales', 'billing'])
})

it('sorts numbers numerically and booleans as 0/1', () => {
  expect(sortBy(rows, 'n').map((r) => r.n)).toEqual([1, 2, 10])
  expect(sortBy(rows, '-on').map((r) => r.on)).toEqual([true, true, false])
})

it('sorts null as empty and does not mutate the input', () => {
  const copy = [...rows]
  expect(sortBy(rows, 'opt').map((r) => r.opt)).toEqual([null, 'a', 'b'])
  expect(rows).toEqual(copy)
})

it('keeps the input order for an empty key', () => {
  expect(sortBy(rows, '').map((r) => r.name)).toEqual(['Sales', 'billing', 'Support'])
})

// Cases from the dashboard lane.
const eRows = [{ name: 'b10', n: 2, on: true }, { name: 'a', n: 10, on: false }, { name: 'b9', n: 1, on: true }]

it('sorts ascending with natural string order', () => {
  expect(sortBy(eRows, 'name').map((r) => r.name)).toEqual(['a', 'b9', 'b10'])
})

it('sorts numbers numerically and descending with a - prefix', () => {
  expect(sortBy(eRows, '-n').map((r) => r.n)).toEqual([10, 2, 1])
})

it('sorts booleans as 0/1 and leaves the input untouched', () => {
  expect(sortBy(eRows, 'on').map((r) => r.on)).toEqual([false, true, true])
  expect(eRows.map((r) => r.name)).toEqual(['b10', 'a', 'b9'])
})

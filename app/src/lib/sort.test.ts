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

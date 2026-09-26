import { sortBy } from './sort'

const rows = [{ name: 'b10', n: 2, on: true }, { name: 'a', n: 10, on: false }, { name: 'b9', n: 1, on: true }]

it('sorts ascending with natural string order', () => {
  expect(sortBy(rows, 'name').map((r) => r.name)).toEqual(['a', 'b9', 'b10'])
})

it('sorts numbers numerically and descending with a - prefix', () => {
  expect(sortBy(rows, '-n').map((r) => r.n)).toEqual([10, 2, 1])
})

it('sorts booleans as 0/1 and leaves the input untouched', () => {
  expect(sortBy(rows, 'on').map((r) => r.on)).toEqual([false, true, true])
  expect(rows.map((r) => r.name)).toEqual(['b10', 'a', 'b9'])
})

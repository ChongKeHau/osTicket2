import { fromLocalInput, toLocalInput, truncate } from './format'

test('suite runs under a fixed non-UTC timezone (TZ=America/Los_Angeles in vite.config.ts)', () => {
  expect(new Date('2026-09-26T10:00:00.000Z').getTimezoneOffset()).toBe(420)
})

test('toLocalInput renders local wall-clock time', () => {
  expect(toLocalInput('2026-09-26T10:00:00.000Z')).toBe('2026-09-26T03:00')
})

test('toLocalInput and fromLocalInput are inverse', () => {
  expect(fromLocalInput(toLocalInput('2026-09-26T10:00:00.000Z'))).toBe('2026-09-26T10:00:00.000Z')
  expect(fromLocalInput(toLocalInput('2026-01-05T23:30:00Z'))).toBe('2026-01-05T23:30:00.000Z')
})

test('truncate keeps short strings and cuts long ones to n characters ending in an ellipsis', () => {
  expect(truncate('short', 60)).toBe('short')
  expect(truncate('x'.repeat(60), 60)).toBe('x'.repeat(60))
  const cut = truncate('y'.repeat(61), 60)
  expect(cut).toHaveLength(60)
  expect(cut).toBe(`${'y'.repeat(59)}…`)
  expect(truncate(null, 10)).toBe('')
  expect(truncate(undefined, 10)).toBe('')
})

import { ApiError } from '../api/client'
import { changedFields, parseId, splitErrors } from './forms'

test('parseId accepts positive integers only', () => {
  expect(parseId('7')).toBe(7)
  expect(parseId('0')).toBeNull()
  expect(parseId('-1')).toBeNull()
  expect(parseId('abc')).toBeNull()
  expect(parseId('1.5')).toBeNull()
  expect(parseId(undefined)).toBeNull()
})

test('changedFields returns only differing keys and ignores array order', () => {
  const before = { name: 'A', is_admin: false, department_ids: [1, 2], primary: 1 }
  const after = { name: 'B', is_admin: false, department_ids: [2, 1], primary: 2 }
  expect(changedFields(before, after)).toEqual({ name: 'B', primary: 2 })
  expect(changedFields(before, { ...before })).toEqual({})
  expect(changedFields(before, { ...before, department_ids: [1] })).toEqual({ department_ids: [1] })
})

test('splitErrors routes known field errors to fields and everything else to the banner', () => {
  const known = ['name', 'is_public']
  const v = new ApiError(400, 'validation_failed', 'request validation failed', { name: 'required' })
  expect(splitErrors(v, known)).toEqual({ fields: { name: 'required' }, banner: null })
  const unknown = new ApiError(400, 'validation_failed', 'request validation failed', { name: 'required', extra: 'bad' })
  expect(splitErrors(unknown, known)).toEqual({ fields: { name: 'required' }, banner: unknown })
  const conflict = new ApiError(409, 'conflict', 'department name already exists')
  expect(splitErrors(conflict, known)).toEqual({ fields: {}, banner: conflict })
  expect(splitErrors(null, known)).toEqual({ fields: {}, banner: null })
})

test('splitErrors sends a known field with an empty message to the banner', () => {
  const blank = new ApiError(400, 'validation_failed', 'request validation failed', { name: '' })
  expect(splitErrors(blank, ['name'])).toEqual({ fields: {}, banner: blank })
})

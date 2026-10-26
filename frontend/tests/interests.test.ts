import assert from 'node:assert/strict'
import test from 'node:test'
import { filterAndSortInterests } from '../src/utils/interests.ts'
import type { Interest } from '../src/utils/interests.ts'

const interests: Interest[] = [
  { id: '00000000-0000-4000-8000-000000000001', keyword: 'Software', weight: 1, is_main: false, is_active: true },
  { id: '00000000-0000-4000-8000-000000000002', keyword: 'backend', weight: 1, is_main: true, is_active: false },
  { id: '00000000-0000-4000-8000-000000000003', keyword: 'go', weight: 1, is_main: true, is_active: true },
  { id: '00000000-0000-4000-8000-000000000004', keyword: 'front end', weight: 1, is_main: false, is_active: false },
]

test('search is case-insensitive, trims spaces, and supports partial matches', () => {
  assert.deepEqual(filterAndSortInterests(interests, '  SOFT  ', { key: 'name', direction: 'asc' }).map((i) => i.id), ['00000000-0000-4000-8000-000000000001'])
  assert.deepEqual(filterAndSortInterests(interests, 'end', { key: 'name', direction: 'asc' }).map((i) => i.id), ['00000000-0000-4000-8000-000000000002', '00000000-0000-4000-8000-000000000004'])
  assert.equal(filterAndSortInterests(interests, 'missing', { key: 'name', direction: 'asc' }).length, 0)
})

test('name sorts in both directions', () => {
  assert.deepEqual(filterAndSortInterests(interests, '', { key: 'name', direction: 'asc' }).map((i) => i.id), ['00000000-0000-4000-8000-000000000002', '00000000-0000-4000-8000-000000000004', '00000000-0000-4000-8000-000000000003', '00000000-0000-4000-8000-000000000001'])
  assert.deepEqual(filterAndSortInterests(interests, '', { key: 'name', direction: 'desc' }).map((i) => i.id), ['00000000-0000-4000-8000-000000000001', '00000000-0000-4000-8000-000000000003', '00000000-0000-4000-8000-000000000004', '00000000-0000-4000-8000-000000000002'])
})

test('role sorts main first or supporting first, then by name', () => {
  assert.deepEqual(filterAndSortInterests(interests, '', { key: 'role', direction: 'asc' }).map((i) => i.id), ['00000000-0000-4000-8000-000000000002', '00000000-0000-4000-8000-000000000003', '00000000-0000-4000-8000-000000000004', '00000000-0000-4000-8000-000000000001'])
  assert.deepEqual(filterAndSortInterests(interests, '', { key: 'role', direction: 'desc' }).map((i) => i.id), ['00000000-0000-4000-8000-000000000004', '00000000-0000-4000-8000-000000000001', '00000000-0000-4000-8000-000000000002', '00000000-0000-4000-8000-000000000003'])
})

test('status sorts active first or inactive first, then by name', () => {
  assert.deepEqual(filterAndSortInterests(interests, '', { key: 'status', direction: 'asc' }).map((i) => i.id), ['00000000-0000-4000-8000-000000000003', '00000000-0000-4000-8000-000000000001', '00000000-0000-4000-8000-000000000002', '00000000-0000-4000-8000-000000000004'])
  assert.deepEqual(filterAndSortInterests(interests, '', { key: 'status', direction: 'desc' }).map((i) => i.id), ['00000000-0000-4000-8000-000000000002', '00000000-0000-4000-8000-000000000004', '00000000-0000-4000-8000-000000000003', '00000000-0000-4000-8000-000000000001'])
})

test('filtering and sorting do not change the source order or flags', () => {
  const original = structuredClone(interests)
  filterAndSortInterests(interests, '', { key: 'status', direction: 'asc' })
  assert.deepEqual(interests, original)
})

test('draft flags affect sorting without dropping edits outside the search', () => {
  const draft = interests.map((interest) => interest.id === '00000000-0000-4000-8000-000000000001' ? { ...interest, is_main: true } : { ...interest })
  assert.deepEqual(filterAndSortInterests(draft, '', { key: 'role', direction: 'asc' }).map((i) => i.id), ['00000000-0000-4000-8000-000000000002', '00000000-0000-4000-8000-000000000003', '00000000-0000-4000-8000-000000000001', '00000000-0000-4000-8000-000000000004'])
  filterAndSortInterests(draft, 'backend', { key: 'name', direction: 'asc' })
  assert.equal(draft.find((interest) => interest.id === '00000000-0000-4000-8000-000000000001')?.is_main, true)
  assert.equal(interests.find((interest) => interest.id === '00000000-0000-4000-8000-000000000001')?.is_main, false)
})

import assert from 'node:assert/strict'
import test from 'node:test'
import { filterAndSortInterests } from '../src/utils/interests.ts'
import type { Interest } from '../src/utils/interests.ts'

const interests: Interest[] = [
  { id: 1, keyword: 'Software', weight: 1, is_main: false, is_active: true },
  { id: 2, keyword: 'backend', weight: 1, is_main: true, is_active: false },
  { id: 3, keyword: 'go', weight: 1, is_main: true, is_active: true },
  { id: 4, keyword: 'front end', weight: 1, is_main: false, is_active: false },
]

test('search is case-insensitive, trims spaces, and supports partial matches', () => {
  assert.deepEqual(filterAndSortInterests(interests, '  SOFT  ', { key: 'name', direction: 'asc' }).map((i) => i.id), [1])
  assert.deepEqual(filterAndSortInterests(interests, 'end', { key: 'name', direction: 'asc' }).map((i) => i.id), [2, 4])
  assert.equal(filterAndSortInterests(interests, 'missing', { key: 'name', direction: 'asc' }).length, 0)
})

test('name sorts in both directions', () => {
  assert.deepEqual(filterAndSortInterests(interests, '', { key: 'name', direction: 'asc' }).map((i) => i.id), [2, 4, 3, 1])
  assert.deepEqual(filterAndSortInterests(interests, '', { key: 'name', direction: 'desc' }).map((i) => i.id), [1, 3, 4, 2])
})

test('role sorts main first or supporting first, then by name', () => {
  assert.deepEqual(filterAndSortInterests(interests, '', { key: 'role', direction: 'asc' }).map((i) => i.id), [2, 3, 4, 1])
  assert.deepEqual(filterAndSortInterests(interests, '', { key: 'role', direction: 'desc' }).map((i) => i.id), [4, 1, 2, 3])
})

test('status sorts active first or inactive first, then by name', () => {
  assert.deepEqual(filterAndSortInterests(interests, '', { key: 'status', direction: 'asc' }).map((i) => i.id), [3, 1, 2, 4])
  assert.deepEqual(filterAndSortInterests(interests, '', { key: 'status', direction: 'desc' }).map((i) => i.id), [2, 4, 3, 1])
})

test('filtering and sorting do not change the source order or flags', () => {
  const original = structuredClone(interests)
  filterAndSortInterests(interests, '', { key: 'status', direction: 'asc' })
  assert.deepEqual(interests, original)
})

test('draft flags affect sorting without dropping edits outside the search', () => {
  const draft = interests.map((interest) => interest.id === 1 ? { ...interest, is_main: true } : { ...interest })
  assert.deepEqual(filterAndSortInterests(draft, '', { key: 'role', direction: 'asc' }).map((i) => i.id), [2, 3, 1, 4])
  filterAndSortInterests(draft, 'backend', { key: 'name', direction: 'asc' })
  assert.equal(draft.find((interest) => interest.id === 1)?.is_main, true)
  assert.equal(interests.find((interest) => interest.id === 1)?.is_main, false)
})

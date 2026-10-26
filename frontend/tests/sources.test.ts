import assert from 'node:assert/strict'
import test from 'node:test'
import { changedSources, filterAndSortSources, normalizeSource, validateSource } from '../src/utils/sources.ts'
import type { Source } from '../src/utils/sources.ts'

const sources: Source[] = [
  { id: '00000000-0000-4000-8000-000000000001', name: 'Science', url: 'https://z.example/rss', is_active: true },
  { id: '00000000-0000-4000-8000-000000000002', name: 'backend', url: 'https://a.example/atom', is_active: false },
  { id: '00000000-0000-4000-8000-000000000003', name: 'World', url: 'https://m.example/feed', is_active: true },
]

test('source search matches names and URLs, ignoring case and surrounding whitespace', () => {
  assert.deepEqual(filterAndSortSources(sources, ' SCI ', { key: 'name', direction: 'asc' }).map((s) => s.id), ['00000000-0000-4000-8000-000000000001'])
  assert.deepEqual(filterAndSortSources(sources, 'ATOM', { key: 'name', direction: 'asc' }).map((s) => s.id), ['00000000-0000-4000-8000-000000000002'])
  assert.equal(filterAndSortSources(sources, 'missing', { key: 'name', direction: 'asc' }).length, 0)
})

test('sources sort by name, URL and status in both directions', () => {
  for (const [key, ascending, descending] of [
    ['name', ['00000000-0000-4000-8000-000000000002', '00000000-0000-4000-8000-000000000001', '00000000-0000-4000-8000-000000000003'], ['00000000-0000-4000-8000-000000000003', '00000000-0000-4000-8000-000000000001', '00000000-0000-4000-8000-000000000002']],
    ['url', ['00000000-0000-4000-8000-000000000002', '00000000-0000-4000-8000-000000000003', '00000000-0000-4000-8000-000000000001'], ['00000000-0000-4000-8000-000000000001', '00000000-0000-4000-8000-000000000003', '00000000-0000-4000-8000-000000000002']],
    ['status', ['00000000-0000-4000-8000-000000000001', '00000000-0000-4000-8000-000000000003', '00000000-0000-4000-8000-000000000002'], ['00000000-0000-4000-8000-000000000002', '00000000-0000-4000-8000-000000000001', '00000000-0000-4000-8000-000000000003']],
  ] as const) {
    assert.deepEqual(filterAndSortSources(sources, '', { key, direction: 'asc' }).map((s) => s.id), ascending)
    assert.deepEqual(filterAndSortSources(sources, '', { key, direction: 'desc' }).map((s) => s.id), descending)
  }
})

test('source filtering and sorting preserve the original list and hidden draft edits', () => {
  const original = structuredClone(sources)
  const draft = sources.map((source) => source.id === '00000000-0000-4000-8000-000000000002' ? { ...source, is_active: true } : { ...source })
  filterAndSortSources(draft, 'Science', { key: 'status', direction: 'asc' })
  assert.deepEqual(sources, original)
  assert.equal(draft[1].is_active, true)
  assert.deepEqual(changedSources(sources, draft).map((s) => s.id), ['00000000-0000-4000-8000-000000000002'])
})

test('change detection includes names, URLs and status but ignores surrounding whitespace', () => {
  assert.deepEqual(changedSources(sources, sources.map((source) => ({ ...source, name: ` ${source.name} ` }))), [])
  for (const update of [{ name: 'Renamed' }, { url: 'https://changed.example/rss' }, { is_active: false }]) {
    const draft = sources.map((source) => source.id === '00000000-0000-4000-8000-000000000001' ? { ...source, ...update } : source)
    assert.deepEqual(changedSources(sources, draft).map((s) => s.id), ['00000000-0000-4000-8000-000000000001'])
  }
})

test('source validation requires a name and an absolute HTTP(S) URL', () => {
  assert.equal(validateSource(sources[0]), null)
  assert.equal(validateSource({ ...sources[0], url: 'http://example.com/rss' }), null)
  assert.ok(validateSource({ ...sources[0], name: '   ' }))
  for (const url of ['', '   ', '/rss', 'example.com/rss', 'ftp://example.com/rss', 'javascript:alert(1)', 'https://']) {
    assert.ok(validateSource({ ...sources[0], url }), url)
  }
  assert.deepEqual(normalizeSource({ name: ' Science ', url: ' https://example.com/rss ', is_active: false }), {
    name: 'Science', url: 'https://example.com/rss', is_active: false,
  })
})

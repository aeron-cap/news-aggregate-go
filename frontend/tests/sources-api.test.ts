import assert from 'node:assert/strict'
import test from 'node:test'
import { fetchSources, insertSources, updateSources } from '../api.ts'

test('source API uses the registered routes and snake_case fields', async (t) => {
  const requests: { url: string; options?: RequestInit }[] = []
  const source = { id: 1, name: 'Science', url: 'https://example.com/rss', is_active: false }
  t.mock.method(globalThis, 'fetch', async (url: string, options?: RequestInit) => {
    requests.push({ url, options })
    return new Response(JSON.stringify(options ? { details: 'Saved' } : [source]), { status: 200 })
  })
  assert.deepEqual(await fetchSources(), [source])
  const fields = { name: source.name, url: source.url, is_active: source.is_active }
  await insertSources([fields])
  await updateSources({ 1: fields })
  assert.deepEqual(requests.map((request) => request.url), [
    'http://localhost:6767/sources', 'http://localhost:6767/sources', 'http://localhost:6767/change-sources',
  ])
  assert.equal(requests[1].options?.method, 'POST')
  assert.deepEqual(requests[1].options?.headers, { 'Content-Type': 'application/json' })
  assert.deepEqual(JSON.parse(requests[1].options?.body as string), [fields])
  assert.deepEqual(JSON.parse(requests[2].options?.body as string), { 1: fields })
})

test('source API reports server errors and handles non-JSON errors', async (t) => {
  t.mock.method(globalThis, 'fetch', async () => new Response(JSON.stringify({ error: 'Source not found', details: 'Missing ID' }), { status: 404 }))
  await assert.rejects(updateSources({}), /Source not found: Missing ID/)
  await assert.rejects(fetchSources(), /Could not load sources/)
  t.mock.method(globalThis, 'fetch', async () => new Response('Unavailable', { status: 503 }))
  await assert.rejects(insertSources([]), /Could not save sources/)
})

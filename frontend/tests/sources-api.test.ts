import assert from 'node:assert/strict'
import test from 'node:test'
import { fetchSources, insertSources, updateSources } from '../api.ts'

test('source API uses the registered routes and snake_case fields', async (t) => {
  const requests: { url: string; options?: RequestInit }[] = []
  const source = { id: 1, name: 'Science', url: 'https://example.com/rss', is_active: false }
  t.mock.method(globalThis, 'fetch', async (url: string, options?: RequestInit) => {
    requests.push({ url, options })
    return new Response(JSON.stringify(options?.method === 'POST' ? { details: 'Saved' } : [source]), { status: 200 })
  })
  assert.deepEqual(await fetchSources(), [source])
  const fields = { name: source.name, url: source.url, is_active: source.is_active }
  await insertSources([fields])
  await updateSources({ 1: fields })
  assert.deepEqual(requests.map((request) => request.url), [
    '/api/set-token', '/api/sources', '/api/sources', '/api/change-sources',
  ])
  assert.ok(requests.every((request) => request.options?.credentials === 'include'))
  assert.equal(requests[2].options?.method, 'POST')
  assert.deepEqual(requests[2].options?.headers, { 'Content-Type': 'application/json' })
  assert.deepEqual(JSON.parse(requests[2].options?.body as string), [fields])
  assert.deepEqual(JSON.parse(requests[3].options?.body as string), { 1: fields })
})

test('source API reports server errors and handles non-JSON errors', async (t) => {
  t.mock.method(globalThis, 'fetch', async () => new Response(JSON.stringify({ error: 'Source not found', details: 'Missing ID' }), { status: 404 }))
  await assert.rejects(updateSources({}), /Source not found: Missing ID/)
  await assert.rejects(fetchSources(), /Could not load sources/)
  t.mock.method(globalThis, 'fetch', async () => new Response('Unavailable', { status: 503 }))
  await assert.rejects(insertSources([]), /Could not save sources/)
})

test('concurrent expired requests share initialization and retry once', async (t) => {
  const calls: string[] = []
  let initialized = false
  t.mock.method(globalThis, 'fetch', async (url: string) => {
    calls.push(url)
    if (url === '/api/set-token') {
      await new Promise((resolve) => setTimeout(resolve, 5))
      initialized = true
      return new Response(JSON.stringify({ details: 'Ready' }), { status: 200 })
    }
    return new Response(JSON.stringify(initialized ? [] : { error: 'Expired' }), {
      status: initialized ? 200 : 401,
    })
  })
  assert.deepEqual(await Promise.all([fetchSources(), fetchSources()]), [[], []])
  assert.equal(calls.filter((url) => url === '/api/set-token').length, 1)
  assert.equal(calls.filter((url) => url === '/api/sources').length, 4)
})

test('persistent unauthorized responses do not loop', async (t) => {
  const calls: string[] = []
  t.mock.method(globalThis, 'fetch', async (url: string) => {
    calls.push(url)
    return new Response(JSON.stringify(url === '/api/set-token' ? { details: 'Ready' } : { error: 'Unauthorized' }), {
      status: url === '/api/set-token' ? 200 : 401,
    })
  })
  await assert.rejects(fetchSources(), /Could not load sources/)
  assert.equal(calls.filter((url) => url === '/api/set-token').length, 1)
  assert.equal(calls.filter((url) => url === '/api/sources').length, 2)
})

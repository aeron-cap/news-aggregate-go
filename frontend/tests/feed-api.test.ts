import assert from 'node:assert/strict'
import test from 'node:test'
import { buildFeed } from '../api.ts'

test('buildFeed calls /build-feed with the session cookie', async (t) => {
  const requests: { url: string; options?: RequestInit }[] = []
  t.mock.method(globalThis, 'fetch', async (url: string, options?: RequestInit) => {
    requests.push({ url, options })
    return new Response(JSON.stringify({ details: 'Feed refreshed successfully' }), { status: 200 })
  })
  await buildFeed()
  assert.deepEqual(requests.map((request) => request.url), ['/api/set-token', '/api/build-feed'])
  assert.equal(requests[1].options?.credentials, 'include')
  assert.equal(requests[1].options?.method, undefined)
})

test('buildFeed reports server errors and handles non-JSON errors', async (t) => {
  t.mock.method(globalThis, 'fetch', async () => new Response(JSON.stringify({ error: 'Failed to build feed' }), { status: 500 }))
  await assert.rejects(buildFeed(), /Failed to build feed/)
  t.mock.method(globalThis, 'fetch', async () => new Response('Bad gateway', { status: 502 }))
  await assert.rejects(buildFeed(), /Could not refresh feed/)
})

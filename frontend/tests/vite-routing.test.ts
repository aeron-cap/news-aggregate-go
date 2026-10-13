import assert from 'node:assert/strict'
import { once } from 'node:events'
import { createServer as createHTTPServer } from 'node:http'
import test from 'node:test'
import { fileURLToPath } from 'node:url'
import { createServer as createViteServer } from 'vite'
import config from '../vite.config.ts'

test('Vite serves api.ts as JavaScript and proxies only API routes', async () => {
  const backendRequests: string[] = []
  const backend = createHTTPServer((request, response) => {
    backendRequests.push(request.url!)
    response.setHeader('Content-Type', 'application/json')
    response.end(JSON.stringify({ ok: true }))
  })
  backend.listen(0, '127.0.0.1')
  await once(backend, 'listening')
  const backendAddress = backend.address()
  assert.ok(backendAddress && typeof backendAddress !== 'string')
  const proxy = Object.fromEntries(Object.entries(config.server!.proxy!).map(([route, options]) => [
    route,
    { ...(typeof options === 'string' ? {} : options), target: `http://127.0.0.1:${backendAddress.port}` },
  ]))
  const vite = await createViteServer({
    ...config,
    configFile: false,
    root: fileURLToPath(new URL('..', import.meta.url)),
    logLevel: 'silent',
    server: {
      host: '127.0.0.1', port: 0,
      proxy,
    },
  })
  try {
    await vite.listen()
    const address = vite.httpServer!.address()
    assert.ok(address && typeof address !== 'string')
    const base = `http://127.0.0.1:${address.port}`
    for (const path of ['/src/main.tsx', '/api.ts']) {
      const module = await fetch(`${base}${path}`)
      assert.equal(module.status, 200)
      assert.match(module.headers.get('Content-Type')!, /javascript/)
      assert.ok((await module.text()).length > 0)
    }
    assert.deepEqual(backendRequests, [])
    const api = await fetch(`${base}/api/healthz?check=1`)
    assert.equal(api.status, 200)
    assert.deepEqual(await api.json(), { ok: true })
    assert.deepEqual(backendRequests, ['/healthz?check=1'])
  } finally {
    await vite.close()
    await new Promise<void>((resolve, reject) => backend.close((error) => error ? reject(error) : resolve()))
  }
})

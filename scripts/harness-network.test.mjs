import { test } from 'node:test'
import assert from 'node:assert/strict'
import net from 'node:net'
import { EventEmitter, once } from 'node:events'
import { createServer } from 'node:http'
import { freePort } from './lib/harness.mjs'

function allocateSequence(t, ports) {
  let opened = 0, closed = 0
  t.mock.method(net, 'createServer', () => {
    const server = new EventEmitter()
    const port = ports[Math.min(opened++, ports.length - 1)]
    server.listen = (requested, host, ready) => {
      assert.equal(requested, 0)
      assert.equal(host, '127.0.0.1')
      queueMicrotask(ready)
    }
    server.address = () => ({ port })
    server.close = done => { closed++; queueMicrotask(done) }
    return server
  })
  return () => ({ opened, closed })
}

test('freePort skips OS-assigned ports blocked by HTTP fetch', async t => {
  const counts = allocateSequence(t, [1719, 1720, 1723, 2049, 5060, 6000, 6667, 10080, 48081])
  assert.equal(await freePort(), 48081)
  assert.deepEqual(counts(), { opened: 9, closed: 9 })
})

test('freePort rejects a port pool that never produces a fetch-compatible port', async t => {
  const counts = allocateSequence(t, [1719])
  await assert.rejects(freePort(), /Cannot allocate a fetch-compatible loopback port/)
  assert.equal(counts().opened, counts().closed)
  assert.ok(counts().opened <= 100, 'allocation must remain bounded')
})

test('an allocated loopback port serves an actual HTTP fetch', { timeout: 5000 }, async () => {
  const port = await freePort()
  const server = createServer((request, response) => response.end('ready'))
  try {
    server.listen(port, '127.0.0.1')
    await once(server, 'listening')
    const response = await fetch(`http://127.0.0.1:${port}/live`, { signal: AbortSignal.timeout(2000) })
    assert.equal(response.status, 200)
    assert.equal(await response.text(), 'ready')
  } finally {
    server.closeAllConnections()
    await new Promise(resolve => server.close(resolve))
  }
})

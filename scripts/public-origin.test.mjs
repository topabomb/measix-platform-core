import {test} from 'node:test'
import assert from 'node:assert/strict'
import http from 'node:http'
import {Worker} from 'node:worker_threads'
import {once} from 'node:events'
import {resolve} from 'node:path'
import {freePort} from './lib/harness.mjs'

test('public harness origin serves Discovery and routes its Runtime path to Relay', async () => {
  const hub = http.createServer((req, res) => {
    res.setHeader('Content-Type', 'application/json')
    res.end(JSON.stringify({runtimeApiBase: '/runtime/v1'}))
  })
  const relay = http.createServer((req, res) => {
    res.setHeader('Content-Type', 'application/json')
    res.end(JSON.stringify({path: req.url, method: req.method, authorization: req.headers.authorization}))
  })
  hub.listen(0, '127.0.0.1')
  relay.listen(0, '127.0.0.1')
  await Promise.all([once(hub, 'listening'), once(relay, 'listening')])
  const spaPort = await freePort()
  const adapterPort = await freePort()
  const worker = new Worker(new URL('./_server-worker.mjs', import.meta.url), {
    execArgv: [],
    workerData: {spaPort, adapterPort, hubPort: hub.address().port, relayPort: relay.address().port, spaDir: resolve('console/dist/spa')},
  })
  try {
    await once(worker, 'message')
    const origin = 'http://127.0.0.1:' + spaPort
    const discovery = await fetch(origin + '/.well-known/measix')
    assert.equal(discovery.status, 200)
    const config = await discovery.json()
    const response = await fetch(origin + config.runtimeApiBase + '/resources/mdl_test/v1/chat/completions?stream=true', {
      method: 'POST',
      headers: {Authorization: 'Bearer synthetic'},
    })
    assert.equal(response.status, 200)
    assert.deepEqual(await response.json(), {
      path: '/runtime/v1/resources/mdl_test/v1/chat/completions?stream=true',
      method: 'POST',
      authorization: 'Bearer synthetic',
    })
    assert.equal((await fetch(origin + '/internal/v1/control/status')).status, 404)
  } finally {
    await worker.terminate()
    hub.closeAllConnections()
    relay.closeAllConnections()
    await Promise.all([new Promise(resolve => hub.close(resolve)), new Promise(resolve => relay.close(resolve))])
  }
})

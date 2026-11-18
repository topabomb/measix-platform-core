import assert from 'node:assert/strict'
import { createHash, randomUUID } from 'node:crypto'
import { once } from 'node:events'
import { readFileSync } from 'node:fs'
import { createServer, request } from 'node:http'
import { join } from 'node:path'
import { setTimeout as pause } from 'node:timers/promises'

const sha256 = bytes => createHash('sha256').update(bytes).digest('hex')

async function waitForObservation(condition, label) {
  for (let attempt = 0; attempt < 300; attempt++) {
    if (condition()) return
    await pause(100)
  }
  throw Error(`Timed out waiting for ${label}`)
}

/** Isolated fixture only. Delay delivery of complete, inspected Runcove 200s
 * so that Relay slot use and cancellation have deterministic observation points. */
export async function startPreviewResponseGate(target, registerServer) {
  const upstream = new URL(target)
  assert.equal(upstream.origin + '/', upstream.href)
  assert.equal(upstream.hostname, '127.0.0.1')
  assert.equal(upstream.protocol, 'http:')
  const gate = { path: '', held: [], requests: 0, lastPreviewResult: null }
  const server = createServer((incoming, outgoing) => {
    if (!incoming.url.startsWith('/') || incoming.url.startsWith('//')) {
      outgoing.writeHead(400).end()
      return
    }
    const url = new URL(incoming.url, upstream)
    const preview = url.pathname.endsWith('/previews')
    const hold = gate.path && incoming.method === 'GET' && preview && url.searchParams.get('path') === gate.path
    if (hold) gate.requests++
    if (preview) gate.lastPreviewResult = { received: true, hold: Boolean(hold) }
    const forwarded = request(upstream, {
      method: incoming.method,
      path: incoming.url,
      // The gate's loopback authority is also the DAV Destination authority.
      headers: incoming.headers,
      agent: false,
    }, remote => {
      if (preview) gate.lastPreviewResult = { status: remote.statusCode, hold: Boolean(hold), space: remote.headers['agent-space-id'] }
      if (!hold || remote.statusCode !== 200) {
        outgoing.writeHead(remote.statusCode, remote.headers)
        remote.pipe(outgoing)
        return
      }
      const hash = createHash('sha256')
      let bytes = 0
      remote.on('data', chunk => { hash.update(chunk); bytes += chunk.length })
      remote.on('end', () => {
        const observation = {
          status: remote.statusCode,
          space: remote.headers['agent-space-id'],
          type: remote.headers['content-type'],
          etag: remote.headers.etag,
          bytes,
          sha256: hash.digest('hex'),
          closed: outgoing.destroyed,
        }
        outgoing.once('close', () => { observation.closed = true })
        gate.held.push(observation)
      })
      remote.on('error', () => outgoing.destroy())
    })
    forwarded.on('error', error => {
      if (preview && !outgoing.destroyed) gate.lastPreviewResult = error.code
      if (!outgoing.headersSent) outgoing.writeHead(502)
      outgoing.end()
    })
    incoming.on('aborted', () => forwarded.destroy())
    outgoing.on('close', () => { if (!outgoing.writableFinished) forwarded.destroy() })
    incoming.pipe(forwarded)
  })
  registerServer(server)
  server.listen(0, '127.0.0.1')
  await once(server, 'listening')
  return { origin: `http://127.0.0.1:${server.address().port}`, gate }
}

export async function publishWorkspaceDAV(api, enabled) {
  let draft = await api('/api/admin/v1/draft')
  assert.equal(draft.content.webdav.length, 1)
  if (draft.content.webdav[0].enabled !== enabled) {
    draft.content.webdav[0].enabled = enabled
    draft = await api('/api/admin/v1/draft', 'PUT', { expectedDraftRevision: draft.draftRevision, content: draft.content })
  }
  const revision = draft.draftRevision
  const validation = await api('/api/admin/v1/draft:validate', 'POST', { expectedDraftRevision: revision })
  assert.equal(validation.valid, true, JSON.stringify(validation.errors))
  const activation = await api('/api/admin/v1/draft:publish', 'POST', {
    expectedDraftRevision: revision,
    acknowledgedWarningCodes: validation.warnings.map(warning => warning.code),
  }, { 'Idempotency-Key': 'idem_' + randomUUID() })
  for (let attempt = 0; attempt < 120; attempt++) {
    const view = await api(`/api/admin/v1/activations/${activation.activationId}`)
    if (view.state === 'COMPLETED') return
    assert.ok(!['FAILED', 'UNKNOWN', 'NEEDS_ATTENTION'].includes(view.state), `DAV activation ${view.state}`)
    await pause(250)
  }
  throw Error('DAV publish did not complete')
}

export async function openPlatformDAV({ api, env, user, accessToken }) {
  if (!accessToken) {
    const enrollment = await api(`/api/admin/v1/users/${user.userId}/enrollments`, 'POST', { expiresInSeconds: 3600 })
    const exchanged = await fetch(`${env.hubBaseURL}/api/client/v1/enrollments/exchange`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ code: enrollment.code, installationId: 'ins_' + randomUUID(), deviceName: 'Isolated DAV preview validation', platform: 'ANDROID', appVersion: 'relay-preview-validation' }),
    })
    assert.equal(exchanged.status, 201)
    accessToken = (await exchanged.json()).accessToken
    assert.ok(accessToken)
  }
  const headers = { Authorization: 'Bearer ' + accessToken }
  const client = await fetch(`${env.hubBaseURL}/api/client/v1/workspace`, { headers })
  assert.equal(client.status, 200)
  const view = await client.json()
  assert.equal(view.agentSpaceId, user.spaceId)
  assert.equal(view.webdav.available, true)
  const spaceURL = new URL(`/runtime/v1/resources/${view.fileExchangeWebdavResourceId}/spaces/${view.agentSpaceId}/dav/`, env.relayPubBaseURL)
  const file = (path = '') => new URL(path, spaceURL)
  const preview = new URL('../previews', spaceURL)
  const send = (url, init = {}) => fetch(url, { ...init, headers: { ...headers, ...init.headers }, redirect: 'manual' })
  return { file, preview, send }
}

async function checkCapabilities(response, space) {
  assert.equal(response.status, 200)
  assert.equal(response.headers.get('Agent-Space-Id'), space)
  assert.equal(response.headers.get('Cache-Control'), 'no-store')
  assert.deepEqual(await response.json(), { schemaVersion: 1, supported: true, sizes: [128, 256, 512], contentType: 'image/webp', mediaKinds: ['IMAGE', 'VIDEO'] })
}

export async function checkColdPlatformDAV({ api, env, users, workspace }) {
  await publishWorkspaceDAV(api, true)
  const outcomes = []
  for (const user of users) {
    const dav = await openPlatformDAV({ api, env, user })
    const resources = `${workspace(user)}/resources?` + new URLSearchParams({ agentSpaceId: user.spaceId })
    const before = (await api(resources)).runtime.value
    assert.equal(before, 'notfound')
    await checkCapabilities(await dav.send(dav.preview, { method: 'OPTIONS' }), user.spaceId)
    const after = (await api(resources)).runtime.value
    assert.equal(after, 'notfound')
    outcomes.push({ space: user.spaceId, before, options: 200, after })
  }
  return outcomes
}

export async function runRelayDAVPreview({ out, env, users, api, raw, command, put, workspace, gate }) {
  const [user, otherUser] = users
  const a = await openPlatformDAV({ api, env, user })
  const b = await openPlatformDAV({ api, env, user: otherUser })
  const checks = []
  const proof = check => { checks.push(check); console.log('PASS RELAY DAV ' + check) }
  const media = join(out, 'agent-space/media')
  const image = 'relay-image.png'
  const video = '01-video.mp4'
  await put(user, image, readFileSync(join(media, 'image.png')))

  async function source(context, path) {
    const response = await context.send(context.file(path), { method: 'HEAD' })
    assert.equal(response.status, 200)
    const etag = response.headers.get('ETag')
    assert.match(etag, /^".+"$/)
    return etag
  }
  const url = (context, path, etag) => {
    const address = new URL(context.preview)
    address.search = new URLSearchParams({ path, expectedEtag: etag, size: '128' })
    return address
  }
  async function inspect(response, space, etag) {
    if (response.status !== 200) {
      throw Error(`Preview returned ${response.status} (last observed upstream ${JSON.stringify(gate.lastPreviewResult)}): ${await response.text()}`)
    }
    assert.equal(response.headers.get('Agent-Space-Id'), space)
    assert.equal(response.headers.get('Preview-Source-ETag'), etag)
    assert.equal(response.headers.get('Cache-Control'), 'no-store')
    assert.equal(response.headers.get('Content-Type'), 'image/webp')
    const bytes = Buffer.from(await response.arrayBuffer())
    const digest = sha256(bytes)
    assert.ok(bytes.length > 12 && bytes.length <= 1024 * 1024)
    assert.equal(Number(response.headers.get('Content-Length')), bytes.length)
    assert.equal(bytes.toString('ascii', 0, 4), 'RIFF')
    assert.equal(bytes.toString('ascii', 8, 12), 'WEBP')
    assert.equal(response.headers.get('ETag'), `"${digest}"`)
    return { digest, etag: `"${digest}"`, bytes: bytes.length }
  }

  let original = await source(a, image)
  const firstURL = url(a, image, original)
  const first = await inspect(await a.send(firstURL), user.spaceId, original)
  const videoETag = await source(a, video)
  const videoProof = await inspect(await a.send(url(a, video, videoETag)), user.spaceId, videoETag)
  assert.notEqual(videoProof.digest, first.digest)
  const head = await a.send(firstURL, { method: 'HEAD' })
  assert.equal(head.status, 200)
  assert.equal(head.headers.get('ETag'), first.etag)
  assert.equal(head.headers.get('Preview-Source-ETag'), original)
  assert.equal(Number(head.headers.get('Content-Length')), first.bytes)
  assert.equal((await head.arrayBuffer()).byteLength, 0)
  for (const method of ['GET', 'HEAD']) {
    const unchanged = await a.send(firstURL, { method, headers: { 'If-None-Match': first.etag } })
    assert.equal(unchanged.status, 304)
    assert.equal(unchanged.headers.get('Agent-Space-Id'), user.spaceId)
    assert.equal(unchanged.headers.get('ETag'), first.etag)
    assert.equal((await unchanged.arrayBuffer()).byteLength, 0)
  }
  let response = await a.send(a.file(image), { method: 'PUT', headers: { 'If-Match': original }, body: readFileSync(join(media, 'changed.png')) })
  assert.equal(response.status, 204)
  response = await a.send(firstURL, { headers: { 'If-None-Match': first.etag } })
  assert.equal(response.status, 412)
  assert.equal((await response.json()).code, 'file_version_conflict')
  original = await source(a, image)
  const currentURL = url(a, image, original)
  const changed = await inspect(await a.send(currentURL), user.spaceId, original)
  assert.notEqual(changed.digest, first.digest)
  proof('real image/video bytes and derived ETags, HEAD/304 without body, source replacement wins over 304')

  for (const denied of [await a.send(b.preview, { method: 'OPTIONS' }), await b.send(currentURL)]) {
    assert.equal(denied.status, 403)
    assert.equal((await denied.json()).forwarded, false)
  }
  const otherETag = await source(b, '00-image.png')
  const missing = await b.send(url(b, image, otherETag))
  assert.equal(missing.status, 404)
  assert.equal((await missing.json()).code, 'file_not_found')
  await checkCapabilities(await b.send(b.preview, { method: 'OPTIONS' }), otherUser.spaceId)
  proof('two enterprise Bearers cannot access each other’s endpoints and a missing source is an ordinary 404')

  const otherURL = url(b, '00-image.png', otherETag)
  const readA = async () => inspect(await a.send(currentURL), user.spaceId, original)
  const readB = async () => inspect(await b.send(otherURL), otherUser.spaceId, otherETag)
  const otherPreview = await readB()
  const parallel = await Promise.all([readA(), readA(), readB(), readB()])
  assert.deepEqual(parallel, [changed, changed, otherPreview, otherPreview])
  proof('four parallel warm Relay preview requests across two spaces preserve both identities and representations')

  const payload = Buffer.alloc(2 * 1024 * 1024, 0x42)
  await put(user, 'relay-range.bin', payload)
  const heldRequests = []
  const controllers = []
  gate.path = image
  let overload, rangeDigest
  try {
    for (let index = 0; index < 16; index++) {
      const controller = new AbortController()
      controllers.push(controller)
      heldRequests.push(a.send(currentURL, { signal: controller.signal }).then(async held => {
        await held.body?.cancel()
        return held.status
      }, () => 'disconnected'))
      await waitForObservation(() => gate.held.length === index + 1, `Runcove response ${index + 1}`)
    }
    assert.ok(gate.held.every(item => !item.closed && item.space === user.spaceId && item.type === 'image/webp' && item.sha256 === changed.digest && item.etag === changed.etag && item.bytes === changed.bytes))
    response = await a.send(currentURL)
    assert.equal(response.status, 429)
    assert.equal(response.headers.get('Retry-After'), '1')
    assert.equal((await response.json()).code, 'preview_busy')
    assert.equal(gate.requests, 16, 'Relay overload must not call Runcove')
    overload = { status: 429, retryAfter: '1', forwarded: false, held: 16 }
    response = await a.send(a.file('relay-range.bin'), { headers: { Range: 'bytes=512-1049087' } })
    assert.equal(response.status, 206)
    const ranged = Buffer.from(await response.arrayBuffer())
    assert.deepEqual(ranged, payload.subarray(512, 1049088))
    rangeDigest = sha256(ranged)
    assert.ok(gate.held.every(item => !item.closed), 'Range must finish with preview slots still occupied')
    proof('16 held real Runcove 200s exhaust only the preview slots; a concurrent ordinary 1 MiB DAV Range remains exact')

    await command(user, 'SET_DAV')
    await waitForObservation(() => gate.held.every(item => item.closed), 'key rotation to close all old Relay requests')
    const outcomes = await Promise.all(heldRequests)
    assert.ok(outcomes.every(status => status === 'disconnected' || status === 502 || status === 504), JSON.stringify(outcomes))
    proof('Core key rotation cancels every in-flight old binding; none of the held old 200s reaches the client')
  } finally {
    gate.path = ''
    for (const controller of controllers) controller.abort()
    await Promise.all(heldRequests)
  }
  await inspect(await a.send(currentURL), user.spaceId, original)
  await checkCapabilities(await b.send(b.preview, { method: 'OPTIONS' }), otherUser.spaceId)
  await publishWorkspaceDAV(api, false)
  for (const address of [a.preview, a.file()]) {
    response = await a.send(address, { method: 'OPTIONS' })
    assert.equal(response.status, 403)
    assert.equal((await response.json()).forwarded, false)
  }
  const hubURL = `${workspace(user)}/thumbnail?` + new URLSearchParams({ agentSpaceId: user.spaceId, path: image, expectedEtag: original, size: '128' })
  const hub = await inspect(await raw(hubURL), user.spaceId, original)
  assert.equal(hub.digest, changed.digest)
  proof('published DAV off disables both Relay endpoints; Admin thumbnail remains available through Hub')
  return { checks, first, video: videoProof, changed, overload, rangeDigest, held: gate.held }
}

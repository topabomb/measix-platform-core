/** Run against the isolated environment created by webdav-acceptance.mjs --keep.
 * Streams a real >4GiB video without collecting file bodies in memory.
 */
import assert from 'node:assert/strict'
import { readFileSync, writeFileSync, createReadStream, statSync } from 'node:fs'
import { resolve, join, dirname } from 'node:path'
import { createHash, randomUUID } from 'node:crypto'
import { setTimeout as pause } from 'node:timers/promises'

const privateFile = resolve(process.argv[2]), video = resolve(process.argv[3])
const env = JSON.parse(readFileSync(privateFile, 'utf8')), output = dirname(privateFile)
const originalDAVRoot = env.davRoot
const existingIndex = process.argv.indexOf('--verify-existing')
const existing = existingIndex < 0 ? undefined : process.argv[existingIndex + 1]
assert.ok(existing === undefined || /^transfer-[0-9]+$/.test(existing), 'expected an existing acceptance folder')
const attempt = existing ?? 'transfer-' + Date.now(), evidenceFile = join(output, (existing ? attempt + '-verify-' + Date.now() : attempt) + '.json')
const checks = [], evidence = { startedAt: new Date().toISOString(), checks, bytes: statSync(video).size }
assert.ok(evidence.bytes >= (process.argv.includes('--512mib') ? 512 * 1024 ** 2 : 2 ** 32 + 1))
const sizeLabel = evidence.bytes > 2 ** 32 ? '>4GiB' : '512MiB'
const proof = (name, detail = {}) => { checks.push({ name, ...detail }); writeFileSync(evidenceFile, JSON.stringify(evidence, null, 2)); console.log('PASS ' + name) }
const save = () => writeFileSync(privateFile, JSON.stringify({ ...env, davRoot: originalDAVRoot }, null, 2), { mode: 0o600 })
async function refresh() {
  const r = await fetch(env.publicOrigin + '/api/client/v1/sessions/refresh', { method: 'POST', headers: { 'Content-Type': 'application/json', 'Idempotency-Key': 'idem_' + randomUUID() }, body: JSON.stringify({ refreshToken: env.user.refreshToken }) })
  assert.equal(r.status, 200); Object.assign(env.user, await r.json()); save()
}
async function admin(path, method = 'GET', body) {
  const r = await fetch(env.publicOrigin + path, { method, headers: { Cookie: env.cookie, Origin: env.publicOrigin, 'X-CSRF-Token': env.session.csrfToken, 'Content-Type': 'application/json', 'Idempotency-Key': 'idem_' + randomUUID() }, body: body === undefined ? undefined : JSON.stringify(body) })
  assert.ok(r.ok, `${method} ${path}: ${r.status} ${await (!r.ok ? r.text() : Promise.resolve(''))}`); return r.status === 204 ? undefined : r.json()
}
async function dav(path = '', method = 'GET', body, headers = {}) {
  return fetch(env.publicOrigin + env.davRoot + path, { method, headers: { Authorization: 'Bearer ' + env.user.accessToken, ...headers }, body, ...(body && typeof body !== 'string' ? { duplex: 'half' } : {}) })
}
async function expect(r, statuses) { assert.ok(statuses.includes(r.status), `expected ${statuses}, got ${r.status}: ${await (!statuses.includes(r.status) ? r.text() : Promise.resolve(''))}`); return r }
async function drain(r) { for await (const _ of r.body ?? []) {} }
const wp = `/api/admin/v1/users/${env.user.userId}/workspace`
async function command(action, extra = {}) {
  const view = await admin(wp), op = await admin(wp, 'POST', { action, expectedRevision: view.bindingRevision, ...extra })
  for (let i = 0; i < 120; i++) { const value = await admin('/api/admin/v1/workspace-operations/' + op.operationId); if (value.state === 'COMPLETED') return admin(wp); assert.ok(!['UNKNOWN', 'NEEDS_ATTENTION'].includes(value.state), JSON.stringify(value)); await pause(250) }
  throw Error('workspace command timeout')
}
async function streamedHash(source, label) {
  const hash = createHash('sha256'); let size = 0, next = 1024 ** 3
  for await (const chunk of source) { hash.update(chunk); size += chunk.length; if (size >= next) { console.log(`${label}: ${Math.floor(size / 1024 ** 2)} MiB`); next += 1024 ** 3 } }
  return { size, sha256: hash.digest('hex') }
}
try {
  const view = await admin(wp)
  let r, source
  if (!existing) {
  await refresh()
  await drain(await expect(await dav(attempt + '/', 'MKCOL'), [201]))
  env.davRoot += attempt + '/'
  await drain(await expect(await dav('', 'OPTIONS'), [200]))
  r = await expect(await dav('', 'PROPFIND', undefined, { Depth: '1' }), [207]); assert.ok((await r.text()).includes(env.davRoot))
  await drain(await expect(await dav('standard-test/', 'MKCOL'), [201]))
  await drain(await expect(await dav('standard-test/%E4%B8%AD%20%25.txt', 'PUT', 'standard DAV'), [201, 204]))
  await drain(await expect(await dav('standard-test/%E4%B8%AD%20%25.txt', 'COPY', undefined, { Destination: env.publicOrigin + env.davRoot + 'standard-test/copy.txt', Overwrite: 'F' }), [201]))
  await drain(await expect(await dav('standard-test/copy.txt', 'MOVE', undefined, { Destination: env.publicOrigin + env.davRoot + 'standard-test/moved.txt', Overwrite: 'F' }), [201]))
  const prop = '<D:propertyupdate xmlns:D="DAV:" xmlns:T="urn:acceptance"><D:set><D:prop><T:note>opaque value</T:note></D:prop></D:set></D:propertyupdate>'
  await drain(await expect(await dav('standard-test/moved.txt', 'PROPPATCH', prop, { 'Content-Type': 'application/xml' }), [207]))
  const lockBody = '<D:lockinfo xmlns:D="DAV:"><D:lockscope><D:exclusive/></D:lockscope><D:locktype><D:write/></D:locktype><D:owner>acceptance</D:owner></D:lockinfo>'
  r = await expect(await dav('standard-test/moved.txt', 'LOCK', lockBody, { 'Content-Type': 'application/xml', Timeout: 'Second-120' }), [200]); const lock = r.headers.get('Lock-Token'); assert.ok(lock); const lockXML = await r.text(); assert.ok(lockXML.includes('lockdiscovery')); assert.ok(!lockXML.includes('/u/measix_'))
  await drain(await expect(await dav('standard-test/moved.txt', 'PUT', 'locked update', { If: `<${env.publicOrigin + env.davRoot}standard-test/moved.txt> (${lock})` }), [200, 201, 204]))
  assert.equal(await (await expect(await dav('standard-test/moved.txt'), [200])).text(), 'locked update')
  await drain(await expect(await dav('standard-test/moved.txt', 'UNLOCK', undefined, { 'Lock-Token': lock }), [204]))
  proof('standard DAV methods, Unicode/percent paths, Destination, tagged If, LOCK/UNLOCK and mapped multistatus')
  const readOnly = await command('SET_DAV_ACCESS', { expectedDAVRevision: view.webdav.desiredBindingRevision, davAccessMode: 'READ_ONLY' })
  assert.equal(readOnly.webdav.accessMode, 'READ_ONLY'); assert.equal(readOnly.bindingRevision, view.bindingRevision)
  await refresh(); await drain(await expect(await dav('denied.txt', 'PUT', 'denied'), [403])); await drain(await expect(await dav('standard-test/moved.txt'), [200]))
  r = await fetch(env.publicOrigin + env.contentPath, { headers: { Cookie: env.cookie } }); assert.equal(r.status, 200); await drain(r)
  await command('SET_DAV_ACCESS', { expectedDAVRevision: readOnly.webdav.desiredBindingRevision, davAccessMode: 'FULL' })
  proof('READ_ONLY enforced at relay, Files remains usable, FULL restored through Activation without changing MCP binding revision')
  source = await streamedHash(createReadStream(video), 'source'); evidence.source = source
  await refresh()
  const uploadStart = Date.now()
  // A real request remains active after its 15-second access JWT expires.
  async function* slowVideo() { let size = 0; for await (const chunk of createReadStream(video, { highWaterMark: 1024 * 1024 })) { yield chunk; size += chunk.length; if (size < 20 * 1024 * 1024) await pause(1000) } }
  r = await expect(await dav('video-over-4gib.mp4', 'PUT', slowVideo(), { 'Content-Length': String(source.size), 'Content-Type': 'video/mp4', 'If-None-Match': '*' }), [201, 204]); await drain(r)
  assert.ok(Date.now() - uploadStart > 15000)
  proof(`DAV streams real ${sizeLabel} MP4 upload through Caddy across access-token expiry`, { durationMs: Date.now() - uploadStart })
  await refresh(); r = await expect(await dav('video-over-4gib.mp4'), [200]); const etag = r.headers.get('ETag'); assert.ok(etag); assert.deepEqual(await streamedHash(r.body, 'DAV download'), source)
  proof(`DAV full ${sizeLabel} download byte count and SHA256 match source`)
  await refresh(); const start = Math.min(2 ** 32 - 32, source.size - 64), end = start + 63
  r = await expect(await dav('video-over-4gib.mp4', 'GET', undefined, { Range: `bytes=${start}-${end}`, 'If-Range': etag }), [206]); assert.equal(r.headers.get('Content-Range'), `bytes ${start}-${end}/${source.size}`)
  assert.deepEqual(Buffer.from(await r.arrayBuffer()), await new Promise((resolve, reject) => { const chunks = []; createReadStream(video, { start, end }).on('data', c => chunks.push(c)).on('end', () => resolve(Buffer.concat(chunks))).on('error', reject) }))
  await drain(await expect(await dav('video-over-4gib.mp4', 'GET', undefined, { Range: `bytes=${source.size}-` }), [416]))
  await drain(await expect(await dav('video-over-4gib.mp4', 'GET', undefined, { 'If-Match': '"stale"' }), [412]))
  proof(`DAV Range at offset ${start}; ETag/If-Range, 412 and 416 preserved`)
  // Keep disk requirements to one video: only remove this attempt's verified file.
  await drain(await expect(await dav('video-over-4gib.mp4', 'DELETE', undefined, { 'If-Match': etag }), [204]))
  } else {
    evidence.priorAttempt = attempt + '.json'
    source = await streamedHash(createReadStream(video), 'source'); evidence.source = source
  }
  const start = Math.min(2 ** 32 - 32, source.size - 64), end = start + 63
  const content = wp + '/content?' + new URLSearchParams({ agentSpaceId: view.agentSpaceId, path: attempt + '/files-video.mp4' })
  if (!existing) { r = await fetch(env.publicOrigin + content, { method: 'PUT', headers: { Cookie: env.cookie, Origin: env.publicOrigin, 'X-CSRF-Token': env.session.csrfToken, 'Content-Length': String(source.size), 'If-None-Match': '*' }, body: createReadStream(video), duplex: 'half' }); await drain(await expect(r, [200])) }
  r = await fetch(env.publicOrigin + content, { headers: { Cookie: env.cookie } }); await expect(r, [200]); const filesEtag = r.headers.get('ETag'); assert.deepEqual(await streamedHash(r.body, 'Files download'), source)
  const preview = content + '&' + new URLSearchParams({ preview: 'video', expectedEtag: filesEtag })
  r = await fetch(env.publicOrigin + preview, { headers: { Cookie: env.cookie, Range: `bytes=${start}-${end}`, 'If-Range': filesEtag } }); await expect(r, [206]); assert.equal(r.headers.get('Content-Type'), 'video/mp4'); assert.match(r.headers.get('Content-Disposition') ?? '', /^inline(?:;|$)/i); await drain(r)
  env.previewPath = preview; save()
  proof(`Console Files ${sizeLabel} ${existing ? 'existing download' : 'upload/download'} hash matches; native-video preview serves Range inline`)
  await pause(4000)
  const usage = await admin('/api/admin/v1/usage/requests?resourceKind=WEBDAV&pageSize=100')
  const rows = usage.items ?? usage.requests ?? []; assert.ok(rows.length > 0, JSON.stringify(usage)); assert.ok(rows.every(row => row.statisticsOnly === true && row.cost === undefined))
  assert.ok(rows.some(row => row.httpMethod === 'PUT' && row.requestBytes >= source.size)); assert.ok(rows.some(row => row.httpMethod === 'GET' && row.responseBytes >= source.size))
  proof('DAV basic transfer statistics reach Hub through durable ingest without costs', { requestRows: rows.length })
  evidence.status = 'PASSED'; evidence.completedAt = new Date().toISOString(); writeFileSync(evidenceFile, JSON.stringify(evidence, null, 2))
} catch (error) { evidence.status = 'FAILED'; evidence.failure = error.message; writeFileSync(evidenceFile, JSON.stringify(evidence, null, 2)); throw error }

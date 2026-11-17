import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createHash } from 'node:crypto'
import { setImmediate as turn } from 'node:timers/promises'
import { WorkspaceThumbnailSession, PreviewError } from './workspace-thumbnail-session.mjs'

const supported = JSON.parse(readFileSync(new URL('../../api/fixtures/workspace/thumbnail-capabilities-supported.json', import.meta.url)))
const unsupported = JSON.parse(readFileSync(new URL('../../api/fixtures/workspace/thumbnail-capabilities-unsupported.json', import.meta.url)))
const bytes = readFileSync(new URL('../../api/fixtures/workspace/thumbnail.webp', import.meta.url))
const endpoint = 'https://core.example/api/client/v1/workspace/thumbnail'
const agentSpaceId = 'spc_fixture', expectedEtag = '"source-1"'
const etag = '"' + createHash('sha256').update(bytes).digest('hex') + '"'
const entry = { path: '中文 space/photo.png', expectedEtag, size: 128 }
const capability = (value = supported, headers = {}) => new Response(JSON.stringify(value), { headers: { 'Content-Type': 'application/json', 'Agent-Space-Id': agentSpaceId, ...headers } })
const picture = (headers = {}, body = bytes) => new Response(body, { headers: { 'Content-Type': 'image/webp', 'Agent-Space-Id': agentSpaceId, 'Preview-Source-ETag': expectedEtag, ETag: etag, 'Content-Length': String(body.length), ...headers } })
const session = fetch => new WorkspaceThumbnailSession({ endpoint, agentSpaceId, fetch })
const protocolError = error => error instanceof PreviewError && error.code === 'preview_protocol_error'

test('one connection shares one OPTIONS promise across concurrent callers and later thumbnails', async () => {
  const calls = []
  const client = session(async (url, options) => {
    calls.push({ url: new URL(url), options })
    return options.method === 'OPTIONS' ? capability({ ...supported, futureField: true }) : picture()
  })
  const values = await Promise.all(Array.from({ length: 12 }, () => client.capabilities()))
  assert.equal(calls.length, 1)
  assert.deepEqual(values[0], supported)
  assert.ok(values.every(value => value === values[0]))
  for (const size of [128, 256]) {
    const result = await client.thumbnail({ ...entry, size })
    assert.deepEqual(Buffer.from(result.bytes), bytes)
    assert.equal(result.etag, etag)
  }
  assert.equal(calls.filter(call => call.options.method === 'OPTIONS').length, 1)
  assert.equal(calls[0].url.search, '?agentSpaceId=spc_fixture')
  assert.equal(calls[1].url.searchParams.get('path'), entry.path)
  assert.equal(calls[1].url.searchParams.get('expectedEtag'), expectedEtag)
  assert.equal(calls[2].url.searchParams.get('size'), '256')
  assert.ok(calls.every(call => call.options.redirect === 'error'))
  client.dispose()
})

test('unsupported is memoized and causes no image request', async () => {
  let calls = 0
  const client = session(async () => { calls++; return capability(unsupported) })
  assert.equal(await client.thumbnail(entry), undefined)
  assert.equal(await client.thumbnail(entry), undefined)
  assert.equal(calls, 1)
  client.dispose()
})

test('capability failures stay failures and are shared rather than probed per file', async () => {
  let calls = 0
  const client = session(async () => { calls++; return new Response('{"code":"preview_timeout"}', { status: 503, headers: { 'Content-Type': 'application/problem+json' } }) })
  for (let index = 0; index < 3; index++) await assert.rejects(client.thumbnail(entry), error => error instanceof PreviewError && error.status === 503 && error.code === 'preview_timeout')
  assert.equal(calls, 1)
  client.dispose()
})

test('aborting one file promptly leaves shared discovery available to another file', async () => {
  let release, discoverySignal
  const calls = []
  const client = session(async (_, options) => {
    calls.push(options.method)
    if (options.method === 'OPTIONS') {
      discoverySignal = options.signal
      return new Promise(resolve => { release = () => resolve(capability()) })
    }
    return picture()
  })
  const controller = new AbortController()
  let aborted = false
  const first = client.thumbnail(entry, { signal: controller.signal }).catch(error => { assert.equal(error.name, 'AbortError'); aborted = true })
  const second = client.thumbnail(entry)
  await turn(); controller.abort(); await turn()
  assert.equal(aborted, true)
  assert.equal(discoverySignal.aborted, false)
  release()
  await first; assert.deepEqual(Buffer.from((await second).bytes), bytes)
  assert.deepEqual(calls, ['OPTIONS', 'GET'])
  client.dispose()
})

test('disposing the connection cancels discovery and prevents later reuse', async () => {
  const client = session(async (_, options) => new Promise((_, reject) => options.signal.addEventListener('abort', () => reject(options.signal.reason), { once: true })))
  const pending = client.capabilities()
  await turn(); client.dispose()
  await assert.rejects(pending, error => error.name === 'AbortError')
  await assert.rejects(client.thumbnail(entry), error => error.name === 'AbortError')
})

test('an immediately cancelled waiter still observes the shared discovery rejection', async () => {
  const controller = new AbortController()
  const client = session(async () => { controller.abort(); throw new Error('transport unavailable') })
  await assert.rejects(client.thumbnail(entry, { signal: controller.signal }), error => error.name === 'AbortError')
  await turn()
  await assert.rejects(client.capabilities(), /transport unavailable/)
  client.dispose()
})

test('capabilities require the supported schema and exact space, with bounded JSON', async t => {
  for (const [name, response] of [
    ['unknown schema', () => capability({ ...supported, schemaVersion: 2 })],
    ['incoherent supported sizes', () => capability({ ...supported, sizes: [128] })],
    ['wrong space', () => capability(supported, { 'Agent-Space-Id': 'other' })],
    ['wrong MIME', () => capability(supported, { 'Content-Type': 'text/html' })],
    ['oversized JSON', () => new Response(' '.repeat(16 * 1024 + 1), { headers: { 'Content-Type': 'application/json', 'Agent-Space-Id': agentSpaceId } })],
  ]) await t.test(name, async () => { const client = session(async () => response()); await assert.rejects(client.capabilities(), protocolError); client.dispose() })
})

test('image results validate identity, source, representation digest and byte bounds', async t => {
  for (const [name, response] of [
    ['wrong space', () => picture({ 'Agent-Space-Id': 'other' })],
    ['wrong source', () => picture({ 'Preview-Source-ETag': '"changed"' })],
    ['wrong digest', () => picture({ ETag: '"wrong"' })],
    ['wrong MIME', () => picture({ 'Content-Type': 'text/html' })],
    ['truncated length', () => picture({ 'Content-Length': String(bytes.length + 1) })],
    ['oversized body', () => picture({}, new Uint8Array(1024 * 1024 + 1))],
  ]) await t.test(name, async () => {
    const client = session(async (_, options) => options.method === 'OPTIONS' ? capability() : response())
    await assert.rejects(client.thumbnail(entry), protocolError); client.dispose()
  })
})

test('a missing file remains a file error and does not invalidate supported capability', async () => {
  let discoveries = 0, files = 0
  const client = session(async (_, options) => {
    if (options.method === 'OPTIONS') { discoveries++; return capability() }
    return files++ ? picture() : new Response('{"code":"file_not_found"}', { status: 404, headers: { 'Content-Type': 'application/problem+json' } })
  })
  await assert.rejects(client.thumbnail(entry), error => error.status === 404 && error.code === 'file_not_found')
  assert.deepEqual(Buffer.from((await client.thumbnail(entry)).bytes), bytes)
  assert.equal(discoveries, 1)
  client.dispose()
})

test('invalid inputs are rejected without issuing a network request', async () => {
  let calls = 0
  const fetch = async () => { calls++; return capability() }
  assert.throws(() => new WorkspaceThumbnailSession({ endpoint: endpoint + '?ignored=true', agentSpaceId, fetch }), TypeError)
  assert.throws(() => new WorkspaceThumbnailSession({ endpoint: 'https://user:secret@core.example/thumbnail', agentSpaceId, fetch }), TypeError)
  const client = session(fetch)
  await assert.rejects(client.thumbnail({ ...entry, size: 300 }), TypeError)
  await assert.rejects(client.thumbnail({ ...entry, expectedEtag: 'W/"weak"' }), TypeError)
  assert.equal(calls, 0)
  client.dispose()
})

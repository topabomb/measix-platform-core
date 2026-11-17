import test from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, readFileSync, writeFileSync, mkdirSync, rmSync, realpathSync } from 'node:fs'
import { join, dirname, basename, resolve } from 'node:path'
import { tmpdir } from 'node:os'
import { createHash } from 'node:crypto'
import { runDemo } from './workspace-thumbnail-demo.mjs'

const capabilities = readFileSync(new URL('../../api/fixtures/workspace/thumbnail-capabilities-supported.json', import.meta.url))
const bytes = readFileSync(new URL('../../api/fixtures/workspace/thumbnail.webp', import.meta.url))
const agentSpaceId = 'spc_fixture', source = '"source-1"'
const etag = '"' + createHash('sha256').update(bytes).digest('hex') + '"'
async function temporary(run) {
  const parent = realpathSync(tmpdir()), directory = mkdtempSync(join(parent, 'core-thumbnail-demo-'))
  try { await run(directory) } finally {
    assert.equal(dirname(realpathSync(directory)), parent)
    assert.equal(realpathSync(directory), resolve(directory))
    assert.ok(basename(directory).startsWith('core-thumbnail-demo-'))
    rmSync(directory, { recursive: true, force: true })
  }
}
function config(directory, items = []) { return { endpoint: 'https://core.example/api/client/v1/workspace/thumbnail', agentSpaceId, accessTokenFile: join(directory, 'token.private'), items } }

test('demo rejects invalid token bytes before fetch and never puts their value in an error', () => temporary(async directory => {
  const value = config(directory)
  const secret = 'private-token-\0-value'
  writeFileSync(value.accessTokenFile, secret)
  let calls = 0
  await assert.rejects(runDemo(value, join(directory, 'output'), async () => { calls++; return new Response(capabilities, { headers: { 'Content-Type': 'application/json', 'Agent-Space-Id': agentSpaceId } }) }), error => error instanceof TypeError && !error.message.includes('private-token'))
  assert.equal(calls, 0)
}))

test('occupied output is preserved before token reading or any request', () => temporary(async directory => {
  const output = join(directory, 'existing')
  mkdirSync(output); writeFileSync(join(output, 'result.json'), 'keep previous evidence')
  let calls = 0
  await assert.rejects(runDemo(config(directory), output, async () => { calls++; throw Error('must not fetch') }), error => error.code === 'EEXIST')
  assert.equal(calls, 0)
  assert.equal(readFileSync(join(output, 'result.json'), 'utf8'), 'keep previous evidence')
}))

test('demo writes selected thumbnails with one discovery while the existing owner refreshes its token', () => temporary(async directory => {
  const value = config(directory, [{ path: '中文 image.png', expectedEtag: source }, { path: 'video.mp4', expectedEtag: source, size: 256 }])
  writeFileSync(value.accessTokenFile, 'token-old')
  const calls = []
  const report = await runDemo(value, join(directory, 'output'), async (url, options) => {
    calls.push({ method: options.method, authorization: options.headers.Authorization, path: new URL(url).searchParams.get('path') })
    if (options.method === 'OPTIONS') { writeFileSync(value.accessTokenFile, 'token-new'); return new Response(capabilities, { headers: { 'Content-Type': 'application/json', 'Agent-Space-Id': agentSpaceId } }) }
    return new Response(bytes, { headers: { 'Content-Type': 'image/webp', 'Agent-Space-Id': agentSpaceId, 'Preview-Source-ETag': source, ETag: etag, 'Content-Length': String(bytes.length) } })
  })
  assert.deepEqual(calls.map(value => value.method), ['OPTIONS', 'GET', 'GET'])
  assert.deepEqual(calls.map(value => value.authorization), ['Bearer token-old', 'Bearer token-new', 'Bearer token-new'])
  assert.equal(calls[1].path, '中文 image.png')
  assert.deepEqual(readFileSync(join(directory, 'output/0000.webp')), bytes)
  assert.deepEqual(readFileSync(join(directory, 'output/0001.webp')), bytes)
  assert.equal(report.items.length, 2)
  const raw = readFileSync(join(directory, 'output/result.json'), 'utf8')
  assert.ok(!raw.includes('token-old') && !raw.includes('token-new'))
}))

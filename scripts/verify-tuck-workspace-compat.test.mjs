import test from 'node:test'
import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { join, resolve, sep } from 'node:path'
import { validateInputs } from './verify-tuck-workspace-compat.mjs'

const evidenceRoot = resolve(import.meta.dirname, '../.artifacts/media-preview')
const commit = 'dc9f10996717fc135b96ae00bc9bd0d0cc5cfd78'
const files = ['internal/tuck/contract/provenance.json', 'frontend/src/shared/enterprise-workspace/EnterpriseWorkspace.vue', 'internal/tuck/contract/client-schemas.json', 'internal/tuck/enterprise_workspace.go']
const hash = bytes => createHash('sha256').update(bytes).digest('hex')
function fixture(t) {
  mkdirSync(evidenceRoot, { recursive: true })
  const directory = mkdtempSync(join(evidenceRoot, 'tuck-compat-preflight-'))
  t.after(() => { assert.ok(resolve(directory).startsWith(evidenceRoot + sep)); rmSync(directory, { recursive: true, force: true }) })
  const tuck = join(directory, 'consumer'), cases = join(directory, 'cases'), output = join(directory, 'result')
  mkdirSync(cases); mkdirSync(tuck)
  const baseline = { commit, workingTreeStatus: '', files: {} }
  for (const name of files) { const path = join(tuck, name); mkdirSync(resolve(path, '..'), { recursive: true }); writeFileSync(path, name); baseline.files[name] = hash(name) }
  const baselineFile = join(directory, 'baseline.json'); writeFileSync(baselineFile, JSON.stringify(baseline))
  const manifest = { schemaVersion: 1, core: { backendSource: 'sha256:' + 'a'.repeat(64), binaries: { hub: 'a'.repeat(64), relay: 'b'.repeat(64) }, adminBuildHash: 'sha256:' + 'c'.repeat(64) }, provider: { image: 'sha256:' + 'd'.repeat(64), expectedSource: 'e'.repeat(64), expectedHelper: 'f'.repeat(64), expectedWorkspace: 'sha256:' + '1'.repeat(64) }, cases: [] }
  for (const [id, path, directoryPath] of [['workspace', '/api/client/v1/workspace', ''], ['files-root', '/api/client/v1/workspace/files?agentSpaceId=spc_test&path=', ''], ['files-nested', '/api/client/v1/workspace/files?agentSpaceId=spc_test&path=nested', 'nested']]) {
    const body = Buffer.from(id === 'workspace' ? '{"agentSpaceId":"spc_test"}' : '{"entries":[]}')
    writeFileSync(join(cases, id + '.json'), body)
    manifest.cases.push({ id, method: 'GET', path, directory: directoryPath, agentSpaceId: 'spc_test', status: 200, contentType: 'application/json', bodyFile: id + '.json', responseSHA256: 'sha256:' + hash(body), byteLength: body.length })
  }
  const manifestFile = join(cases, 'manifest.json')
  const saveManifest = () => writeFileSync(manifestFile, JSON.stringify(manifest))
  saveManifest()
  const state = { commit, status: '' }
  const git = (_repo, args) => args.includes('rev-parse') ? state.commit : state.status
  return { options: { tuck, baseline: baselineFile, manifest: manifestFile, output }, state, git, manifest, saveManifest }
}

test('preflight accepts pinned consumer and hashed Client captures without creating output', t => {
  const f = fixture(t); const result = validateInputs(f.options, { git: f.git })
  assert.equal(result.before.commit, commit)
  assert.equal(existsSync(f.options.output), false)
})
test('occupied evidence is never overwritten', t => {
  const f = fixture(t); mkdirSync(f.options.output); const marker = join(f.options.output, 'keep'); writeFileSync(marker, 'original')
  assert.throws(() => validateInputs(f.options, { git: f.git }), /occupied/i)
  assert.equal(readFileSync(marker, 'utf8'), 'original')
})
test('consumer HEAD status and each pinned file are checked before output exists', t => {
  for (const change of ['commit', 'status', 'file']) {
    const f = fixture(t)
    if (change === 'commit') f.state.commit = '0'.repeat(40)
    if (change === 'status') f.state.status = ' M internal/tuck/enterprise_workspace.go'
    if (change === 'file') writeFileSync(join(f.options.tuck, files[0]), 'changed')
    assert.throws(() => validateInputs(f.options, { git: f.git }), /consumer|baseline/i)
    assert.equal(existsSync(f.options.output), false)
  }
})
test('capture status hash and length are independently guarded', t => {
  for (const change of ['status', 'hash', 'length']) {
    const f = fixture(t), capture = f.manifest.cases[0]
    if (change === 'status') capture.status = 503
    if (change === 'hash') capture.responseSHA256 = 'sha256:' + '0'.repeat(64)
    if (change === 'length') capture.byteLength++
    f.saveManifest()
    assert.throws(() => validateInputs(f.options, { git: f.git }), /capture|response/i)
    assert.equal(existsSync(f.options.output), false)
  }
})
test('Admin or escaped capture paths cannot masquerade as current Client responses', t => {
  for (const change of ['admin', 'bodyFile']) {
    const f = fixture(t)
    if (change === 'admin') f.manifest.cases[0].path = '/api/admin/v1/users/usr_test/workspace'
    else f.manifest.cases[0].bodyFile = '../baseline.json'
    f.saveManifest()
    assert.throws(() => validateInputs(f.options, { git: f.git }), /client|capture/i)
    assert.equal(existsSync(f.options.output), false)
  }
})

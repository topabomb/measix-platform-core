import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, cpSync, rmSync, symlinkSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { tmpdir } from 'node:os'
import { spawnSync } from 'node:child_process'
import { bytesHash, platformContractIdentity, validateAndroidRelease, validateConsumerEvidence, compatibilitySummary, payloadHash, pinnedFile } from './lib/release-contract.mjs'
import { parseVerificationArgs } from './verify-preview-contract.mjs'

test('builder refuses occupied product identity before generating, contacting peers or changing output', () => {
  const temp = mkdtempSync(join(tmpdir(), 'measix-release-occupied-'))
  try {
    mkdirSync(join(temp, 'scripts'), { recursive: true })
    cpSync(resolve(import.meta.dirname, 'lib'), join(temp, 'scripts/lib'), { recursive: true })
    cpSync(resolve(import.meta.dirname, 'build-preview-release.mjs'), join(temp, 'scripts/build-preview-release.mjs'))
    cpSync(resolve(import.meta.dirname, 'verify-preview-contract.mjs'), join(temp, 'scripts/verify-preview-contract.mjs'))
    const stage = join(temp, '.artifacts/releases/measix-core-0.2.0-preview.22-linux-arm64')
    mkdirSync(stage, { recursive: true })
    writeFileSync(join(stage, 'immutable.txt'), 'published bytes')
    const result = spawnSync(process.execPath, [join(temp, 'scripts/build-preview-release.mjs'), '0.2.0-preview.22', '--android-release', 'record.json', '--evidence', 'proof.json'], { encoding: 'utf8' })
    assert.notEqual(result.status, 0)
    assert.match(result.stderr, /Release identity already exists or is occupied/)
    assert.equal(readFileSync(join(stage, 'immutable.txt'), 'utf8'), 'published bytes')
  } finally { rmSync(temp, { recursive: true, force: true }) }
})

test('ordinary packaging needs no Android record; partial opt-in evidence still fails', () => {
  const temp = mkdtempSync(join(tmpdir(), 'measix-release-default-'))
  try {
    const root = join(temp, 'core')
    mkdirSync(join(root, 'scripts'), { recursive: true })
    cpSync(resolve(import.meta.dirname, 'lib'), join(root, 'scripts/lib'), { recursive: true })
    for (const name of ['build-preview-release.mjs', 'verify-preview-contract.mjs']) cpSync(join(import.meta.dirname, name), join(root, 'scripts', name))
    const run = args => spawnSync(process.execPath, [join(root, 'scripts/build-preview-release.mjs'), '0.2.0-preview.23', ...args], { encoding: 'utf8', windowsHide: true })
    for (const args of [[], ['--candidate'], ['--mode', 'core']]) {
      const result = run(args)
      assert.notEqual(result.status, 0)
      assert.match(result.stderr, /Portal repository not found/, JSON.stringify(args))
      assert.doesNotMatch(result.stderr, /requires --android-release/)
    }
    for (const args of [['--android-release', 'record.json'], ['--evidence', 'proof.json']]) {
      assert.match(run(args).stderr, /requires --android-release RECORD --evidence PROOF/)
    }
    const stage = join(root, '.artifacts/releases/measix-core-0.2.0-preview.23-linux-arm64-candidate')
    mkdirSync(stage, { recursive: true })
    writeFileSync(join(stage, 'immutable.txt'), 'candidate bytes')
    assert.match(run([]).stderr, /Release identity already exists or is occupied/)
    assert.equal(readFileSync(join(stage, 'immutable.txt'), 'utf8'), 'candidate bytes')
  } finally { rmSync(temp, { recursive: true, force: true }) }
})

function associationFixture() {
  const root = mkdtempSync(join(tmpdir(), 'measix-release-association-'))
  const put = (name, value) => { mkdirSync(resolve(root, name, '..'), { recursive: true }); writeFileSync(join(root, name), value) }
  const json = (name, value) => put(name, JSON.stringify(value))
  const item = name => ({ path: name, sha256: bytesHash(readFileSync(join(root, name))) })
  const baseline = { platformContractVersion: 2, supportedPlatformContractVersions: [2], coreBaselineVersion: '0.2.0-preview.23', baseline: 'original' }
  const identity = platformContractIdentity(Buffer.from(JSON.stringify(baseline)))
  const schema = 'openapi: 3.0.3\ncomponents: {}\n'
  put('contracts/platform/client-control.openapi.yaml', schema)
  json('contracts/platform/protocol-baseline.json', baseline)
  json('contracts/platform/manifest.json', { ...identity, generated: true, format: 'openapi-3.0.3-client-schema-only', sourceHash: bytesHash(schema) })
  put('contracts/PlatformWire.kt', `// Core source SHA256 (LF): ${bytesHash(schema).slice(7)}\n`)
  put('contracts/portal/vector.json', '{}')
  json('contracts/portal/manifest.json', { ...identity, bridgeVersion: 3, artifacts: { 'vector.json': { sha256: bytesHash('{}').slice(7) } } })
  put('app.apk', 'fixed unit-test artifact, not a native APK')
  const record = {
    formatVersion: 1, product: 'MEASIX Android', status: 'candidate', sourceCommit: 'a'.repeat(40), sourceDirty: false,
    applicationId: 'net.weero.measix.pilot', versionName: '0.0.20', versionCode: 20, ...identity, snapshotSchemaVersions: [4, 5],
    contracts: { directory: 'contracts', platformManifest: item('contracts/platform/manifest.json'), portalManifest: item('contracts/portal/manifest.json'), wire: item('contracts/PlatformWire.kt') },
    artifacts: [{ ...item('app.apk'), abi: 'arm64-v8a', variant: 'debug', signingCertificateSha256: 'sha256:' + '1'.repeat(64) }],
  }
  json('android-release.json', record)
  const core = { version: '0.2.0-preview.24', sourceCommit: 'b'.repeat(40), architectureCommit: 'c'.repeat(40), portalCommit: 'd'.repeat(40), baselineHash: 'sha256:' + '2'.repeat(64), buildHash: 'sha256:' + '3'.repeat(64) }
  const checks = ['identity', 'snapshot', 'runtime', 'portal'].map(id => {
    put(id + '.xml', `<testsuite tests="1" failures="0" errors="0" skipped="0"><testcase name="${id}"/></testsuite>`)
    put(id + '.log', 'fixture command output')
    return { id, command: 'consumer-tests ' + id, exitCode: 0, report: item(id + '.xml'), log: item(id + '.log') }
  })
  const proof = { formatVersion: 1, suite: 'android-core-consumer', result: 'PASS', sourceDirty: false, core: { ...core }, android: { releaseHash: item('android-release.json').sha256, sourceCommit: record.sourceCommit, apkSha256: record.artifacts[0].sha256 }, checks }
  json('evidence.json', proof)
  return { root, record, identity, core, proof, json, put, item, recordFile: join(root, 'android-release.json'), proofFile: join(root, 'evidence.json'), cleanup: () => rmSync(root, { recursive: true, force: true }) }
}

test('independent Core release verifies the original Android materials without requiring latest material equality', () => {
  const f = associationFixture()
  try {
    const newer = { ...f.identity, baselineHash: f.core.baselineHash }
    const android = validateAndroidRelease(f.recordFile, newer, 'independent')
    assert.equal(validateConsumerEvidence(f.proofFile, android, f.core).artifact.sha256, f.record.artifacts[0].sha256)
    assert.throws(() => validateAndroidRelease(f.recordFile, newer, 'joint'), /Joint/)
    const summary = compatibilitySummary({ version: f.core.version, ...newer, publicationStatus: 'UNVERIFIED_CANDIDATE', compatibility: { verifiedAndroid: [] } })
    assert.match(summary, /未附自动验证结果/)
    const verified = compatibilitySummary({ version: f.core.version, ...newer, publicationStatus: 'VERIFIED_PREVIEW', compatibility: { verifiedAndroid: [{ ...f.record, apkSha256: f.record.artifacts[0].sha256 }] } })
    assert.match(verified, /\| 0\.0\.20（20） \| 0\.2\.0-preview\.24 \| 已验证 \|/)
    assert.match(verified, new RegExp(f.record.artifacts[0].sha256))
  } finally { f.cleanup() }
})

test('matching contract numbers cannot bypass changed APK, broken original hashes or a falsely declared formal release', () => {
  for (const kind of ['apk', 'manifest', 'formal', 'tag', 'support', 'snapshot', 'escape']) {
    const f = associationFixture()
    try {
      if (kind === 'apk') f.put('app.apk', 'different content, same product version')
      if (kind === 'manifest') f.put('contracts/platform/manifest.json', '{}')
      if (kind === 'formal') { f.record.status = 'released'; f.record.tag = 'v0.0.20' }
      if (kind === 'tag') { f.record.status = 'released'; f.record.tag = 'v0.0.21' }
      if (kind === 'support') f.record.supportedPlatformContractVersions = [1]
      if (kind === 'snapshot') f.record.snapshotSchemaVersions = [4, 4]
      if (kind === 'escape') f.record.artifacts[0].path = '../outside.apk'
      f.json('android-release.json', f.record)
      assert.throws(() => validateAndroidRelease(f.recordFile, f.identity), undefined, kind)
    } finally { f.cleanup() }
  }
})

test('consumer proof requires exact source/build/APK, report bytes and actual successful executions without skips', () => {
  for (const kind of ['build', 'source', 'apk', 'record', 'hash', 'failed', 'skip', 'exit', 'missing', 'empty']) {
    const f = associationFixture()
    try {
      const android = validateAndroidRelease(f.recordFile, f.identity)
      if (kind === 'build') f.proof.core.buildHash = 'sha256:' + '9'.repeat(64)
      if (kind === 'source') f.proof.core.sourceCommit = '9'.repeat(40)
      if (kind === 'apk') f.proof.android.apkSha256 = 'sha256:' + '9'.repeat(64)
      if (kind === 'record') f.proof.android.releaseHash = 'sha256:' + '9'.repeat(64)
      if (kind === 'hash') f.put('snapshot.log', 'changed after capture')
      if (['failed', 'skip', 'empty'].includes(kind)) {
        f.put('snapshot.xml', `<testsuite tests="${kind === 'empty' ? 0 : 1}" failures="${kind === 'failed' ? 1 : 0}" errors="0" skipped="${kind === 'skip' ? 1 : 0}"><testcase name="snapshot"/></testsuite>`)
        f.proof.checks[1].report = f.item('snapshot.xml')
      }
      if (kind === 'exit') f.proof.checks[0].exitCode = 1
      if (kind === 'missing') f.proof.checks.pop()
      f.json('evidence.json', f.proof)
      const expected = { build: /Core buildHash mismatch/, source: /Core sourceCommit mismatch/, apk: /APK mismatch/, record: /release\/source mismatch/, hash: /hash mismatch/, failed: /JUnit/, skip: /JUnit/, exit: /command\/exit failed/, missing: /checks incomplete/, empty: /JUnit/ }
      assert.throws(() => validateConsumerEvidence(f.proofFile, android, f.core), expected[kind], kind)
    } finally { f.cleanup() }
  }
})

test('invalid platform identities and ambiguous verification flags fail closed', () => {
  const value = { platformContractVersion: 2, supportedPlatformContractVersions: [2], coreBaselineVersion: '0.2.0-preview.23' }
  for (const bad of [{ ...value, platformContractVersion: 0 }, { ...value, supportedPlatformContractVersions: [1] }, { ...value, supportedPlatformContractVersions: [2, 2] }, { ...value, coreBaselineVersion: '../overwrite' }]) assert.throws(() => platformContractIdentity(JSON.stringify(bad)))
  assert.throws(() => parseVerificationArgs(['--mode', 'core', '--evidence', 'x']))
  assert.throws(() => parseVerificationArgs(['--mode', 'joint', '--mode', 'core']))
})

test('consumer build identity detects changed bundled binary or asset bytes', () => {
  const f = associationFixture()
  try {
    for (const directory of ['bin', 'assets', 'deploy']) mkdirSync(join(f.root, directory))
    f.put('bin/control-hub', 'original binary')
    const before = payloadHash(f.root)
    f.put('bin/control-hub', 'another binary')
    assert.notEqual(payloadHash(f.root), before)
  } finally { f.cleanup() }
})

test('artifact paths cannot leave the record directory through an external link back inside it', () => {
  const f = associationFixture()
  try {
    symlinkSync(join(f.root, 'contracts'), join(f.root, 'external-link'), process.platform === 'win32' ? 'junction' : 'dir')
    assert.throws(() => pinnedFile(join(f.root, 'contracts'), '../external-link/platform/protocol-baseline.json'), /escapes record directory/)
  } finally { f.cleanup() }
})

test('consumer evidence rechecks original Android files changed after preflight', () => {
  for (const kind of ['record', 'apk', 'material']) {
    const f = associationFixture()
    try {
      const android = validateAndroidRelease(f.recordFile, f.identity)
      if (kind === 'record') { f.record.versionCode = 21; f.json('android-release.json', f.record) }
      if (kind === 'apk') f.put('app.apk', 'changed during Core build')
      if (kind === 'material') f.put('contracts/portal/vector.json', '{"changed":true}')
      assert.throws(() => validateConsumerEvidence(f.proofFile, android, f.core), /changed after preflight|hash mismatch/, kind)
    } finally { f.cleanup() }
  }
})

test('Client and Portal exports declare the same platform semantics and baseline source', () => {
  const root = resolve(import.meta.dirname, '..')
  const client = JSON.parse(readFileSync(join(root, 'api/generated/android/manifest.json')))
  const portal = JSON.parse(readFileSync(join(root, 'api/generated/android/portal/manifest.json')))
  assert.equal(client.platformContractVersion, 2)
  assert.deepEqual(client.supportedPlatformContractVersions, [2])
  for (const field of ['platformContractVersion', 'supportedPlatformContractVersions', 'coreBaselineVersion', 'baselineHash']) assert.deepEqual(client[field], portal[field])
  assert.match(client.baselineHash, /^sha256:[a-f0-9]{64}$/)
})

test('independent verification entry point uses the pinned Android bundle without an Android checkout', () => {
  const f = associationFixture()
  try {
    const coreRoot = join(f.root, 'measix-platform-core')
    const portalRoot = join(f.root, 'measix-enterprise-portal')
    const architectureRoot = join(f.root, 'measix-architecture')
    for (const root of [coreRoot, portalRoot, architectureRoot]) mkdirSync(root)
    const source = 'openapi: 3.0.3\ncomponents: {}\n'
    const names = ['api/client/client-control.openapi.yaml', 'api/admin/admin.openapi.yaml', 'api/internal/relay-control.openapi.yaml', 'api/internal/usage-ingest.openapi.yaml']
    for (const name of names) f.put('measix-platform-core/' + name, source)
    const baseline = { platformContractVersion: 2, supportedPlatformContractVersions: [2], coreBaselineVersion: '0.2.0-preview.23', baseline: 'newer-fixed-core-materials', policy: 'deliberate', documents: Object.fromEntries(names.map(name => [name, bytesHash(source)])) }
    const identity = platformContractIdentity(JSON.stringify(baseline))
    f.json('measix-platform-core/api/protocol-baseline.json', baseline)
    f.json('measix-platform-core/api/generated/android/protocol-baseline.json', baseline)
    f.put('measix-platform-core/api/generated/android/client-control.openapi.yaml', source)
    f.json('measix-platform-core/api/generated/android/manifest.json', { ...identity, generated: true, format: 'openapi-3.0.3-client-schema-only', sourceHash: bytesHash(source) })
    f.put('measix-platform-core/api/portal/vector.json', '{}')
    f.put('measix-platform-core/api/generated/android/portal/vector.json', '{}')
    const artifact = { source: 'api/portal/vector.json', sha256: bytesHash('{}').slice(7) }
    f.json('measix-platform-core/api/generated/android/portal/manifest.json', { ...identity, bridgeVersion: 3, artifacts: { 'vector.json': artifact } })
    f.json('measix-enterprise-portal/src/api/contract.json', { artifacts: { 'vector.json': { ...artifact, source: 'measix-platform-core/' + artifact.source } } })
    cpSync(import.meta.dirname, join(coreRoot, 'scripts'), { recursive: true })
    for (const [root, key] of [[coreRoot, 'sourceCommit'], [portalRoot, 'portalCommit'], [architectureRoot, 'architectureCommit']]) {
      for (const args of [['init', '-q'], ['-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', 'commit', '--allow-empty', '-qm', 'test source identity']]) {
        const result = spawnSync('git', args, { cwd: root, encoding: 'utf8', windowsHide: true })
        assert.equal(result.status, 0, result.stderr)
      }
      f.proof.core[key] = spawnSync('git', ['rev-parse', 'HEAD'], { cwd: root, encoding: 'utf8', windowsHide: true }).stdout.trim()
    }
    f.proof.core.baselineHash = identity.baselineHash
    f.json('evidence.json', f.proof)
    const result = spawnSync(process.execPath, [join(coreRoot, 'scripts/verify-preview-contract.mjs'), '--mode', 'independent', '--version', f.core.version, '--android-release', f.recordFile, '--evidence', f.proofFile], { encoding: 'utf8', windowsHide: true })
    assert.equal(result.status, 0, result.stderr)
    assert.match(result.stdout, /independent; consumer evidence checked/)
  } finally { f.cleanup() }
})

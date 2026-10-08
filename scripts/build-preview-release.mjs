#!/usr/bin/env node
import { createHash } from 'node:crypto'
import { chmodSync, cpSync, existsSync, mkdirSync, readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { basename, dirname, join, relative, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { spawnSync } from 'node:child_process'
import { snapshotVersions } from './lib/harness.mjs'
import { releaseAndroidRoot } from './lib/release-paths.mjs'
import { assertReleaseOutputAvailable, compatibilitySummary, payloadHash, pinnedFile, validateConsumerEvidence } from './lib/release-contract.mjs'
import { parseVerificationArgs, verifyPreviewContract } from './verify-preview-contract.mjs'

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const PORTAL = resolve(ROOT, '..', 'measix-enterprise-portal')
const ARCHITECTURE = resolve(ROOT, '..', 'measix-architecture')
const ANDROID = releaseAndroidRoot(ROOT)
const version = process.argv[2]
if (!version || !/^[0-9A-Za-z][0-9A-Za-z._-]{0,63}$/.test(version)) fail('Usage: node scripts/build-preview-release.mjs <version>')
const explicitCandidate = process.argv.slice(3).includes('--candidate')
if (process.argv.slice(3).filter(arg => arg === '--candidate').length > 1) fail('Duplicate --candidate')
const options = parseVerificationArgs(process.argv.slice(3).filter(arg => arg !== '--candidate'))
if (options.version) fail('Specify the product version only as the first positional argument')
// Ordinary packaging is independent of Android evidence. Existing evidence flags
// opt into the stricter bundled verification workflow.
const candidate = explicitCandidate || (!options.androidRelease && !options.evidence && (!options.mode || options.mode === 'core'))
options.mode ??= candidate ? 'core' : 'independent'
options.version = version
const packageName = `measix-core-${version}-linux-arm64${candidate ? '-candidate' : ''}`
const outputDir = join(ROOT, '.artifacts', 'releases')
const stage = join(outputDir, packageName)
const archive = join(outputDir, `${packageName}.tar.gz`)
try { assertReleaseOutputAvailable(stage, archive) } catch (error) { fail(error.message) }
if (candidate && (options.mode !== 'core' || options.androidRelease || options.evidence)) fail('Candidate is core-only and cannot claim Android verification')
if (!candidate && (options.mode === 'core' || !options.androidRelease || !options.evidence)) fail('Bundled verification requires --android-release RECORD --evidence PROOF; omit both for ordinary Core packaging')
if (!existsSync(join(PORTAL, 'package.json'))) fail(`Portal repository not found: ${PORTAL}`)
if (git(ROOT, ['status', '--porcelain']).trim()) fail('Core worktree must be clean before building a release')
if (git(PORTAL, ['status', '--porcelain']).trim()) fail('Portal worktree must be clean before building a release')
if (git(ARCHITECTURE, ['status', '--porcelain']).trim()) fail('Architecture worktree must be clean before building a release')
if (options.mode === 'joint' && git(ANDROID, ['status', '--porcelain']).trim()) fail('Android worktree must be clean before building a release')
run('node', ['scripts/checks.mjs', 'generate'], ROOT)
if (git(ROOT, ['status', '--porcelain']).trim()) fail('Generated contracts or dependencies drift from committed sources')
run('pnpm', ['generate:api'], PORTAL)
if (git(PORTAL, ['status', '--porcelain']).trim()) fail('Portal generated contracts drift from committed sources')
const verification = verifyPreviewContract(ROOT, options)
mkdirSync(outputDir, { recursive: true })
// Atomic reservation: a concurrent builder cannot reuse this output identity.
mkdirSync(stage)
mkdirSync(join(stage, 'bin'), { recursive: true })
mkdirSync(join(stage, 'assets'), { recursive: true })
mkdirSync(join(stage, 'deploy'), { recursive: true })

run('pnpm', ['-C', 'console', 'build'], ROOT)
run('pnpm', ['build'], PORTAL)
run('go', ['build', '-trimpath', '-ldflags', `-s -w -X main.buildVersion=${version}`, '-o', join(stage, 'bin', 'control-hub'), './cmd/control-hub'], join(ROOT, 'backend'), { CGO_ENABLED: '0', GOOS: 'linux', GOARCH: 'arm64' })
run('go', ['build', '-trimpath', '-ldflags', `-s -w -X main.buildVersion=${version}`, '-o', join(stage, 'bin', 'runtime-relay'), './cmd/runtime-relay'], join(ROOT, 'backend'), { CGO_ENABLED: '0', GOOS: 'linux', GOARCH: 'arm64' })

cpSync(join(ROOT, 'console', 'dist', 'spa'), join(stage, 'assets', 'admin'), { recursive: true })
cpSync(join(PORTAL, 'dist'), join(stage, 'assets', 'portal'), { recursive: true })
cpSync(join(ROOT, 'deploy', 'preview'), join(stage, 'deploy'), { recursive: true })
for (const path of walk(join(stage, 'deploy')).filter(path => path.endsWith('.sh'))) chmodSync(path, 0o755)
chmodSync(join(stage, 'bin', 'control-hub'), 0o755)
chmodSync(join(stage, 'bin', 'runtime-relay'), 0o755)

const protocolFiles = [
  'api/admin/admin.openapi.yaml',
  'api/client/client-control.openapi.yaml',
  'api/internal/relay-control.openapi.yaml',
  'api/internal/usage-ingest.openapi.yaml',
]
const release = {
  formatVersion: 1,
  product: 'MEASIX Core S0.2 Preview',
  version,
  ...verification.identity,
  publicationStatus: candidate ? 'UNVERIFIED_CANDIDATE' : 'VERIFIED_PREVIEW',
  buildHash: payloadHash(stage),
  target: { os: 'linux', arch: 'arm64', platform: 'NVIDIA DGX Spark' },
  builtAt: new Date().toISOString(),
  source: {
    architectureCommit: git(ARCHITECTURE, ['rev-parse', 'HEAD']).trim(),
    coreCommit: git(ROOT, ['rev-parse', 'HEAD']).trim(),
    portalCommit: git(PORTAL, ['rev-parse', 'HEAD']).trim(),
    androidCommit: verification.association?.release.record.sourceCommit ?? null,
  },
  protocols: Object.fromEntries(protocolFiles.map(path => [path, `sha256:${sha256(join(ROOT, path))}`])),
  compatibility: compatibilityEvidence(),
  schemaMigrationIdentity: migrationIdentity(),
}
if (verification.association) {
  const association = verification.association
  association.evidence = validateConsumerEvidence(resolve(options.evidence), association.release, {
    version, sourceCommit: release.source.coreCommit, architectureCommit: release.source.architectureCommit,
    portalCommit: release.source.portalCommit, baselineHash: release.baselineHash, buildHash: release.buildHash,
  })
  const a = association.release.record, apk = association.evidence.artifact
  release.compatibility.verifiedAndroid = [{
    applicationId: a.applicationId, versionName: a.versionName, versionCode: a.versionCode, status: a.status,
    sourceCommit: a.sourceCommit, platformContractVersion: a.platformContractVersion,
    supportedPlatformContractVersions: a.supportedPlatformContractVersions, coreBaselineVersion: a.coreBaselineVersion,
    apkSha256: apk.sha256, abi: apk.abi, variant: apk.variant, signingCertificateSha256: apk.signingCertificateSha256,
    releaseRecordHash: association.release.recordHash, evidenceHash: association.evidence.evidenceHash,
  }]
  const evidenceDir = join(stage, 'compatibility/consumer')
  mkdirSync(evidenceDir, { recursive: true })
  cpSync(resolve(options.androidRelease), join(stage, 'compatibility/android-release.json'))
  cpSync(resolve(options.evidence), join(evidenceDir, 'evidence.json'))
  for (const check of association.evidence.proof.checks) {
    for (const item of [check.report, check.log]) {
      const original = pinnedFile(association.evidence.base, item.path)
      const copy = resolve(evidenceDir, item.path)
      mkdirSync(dirname(copy), { recursive: true })
      cpSync(original, copy)
    }
  }
}
writeFileSync(join(stage, 'release.json'), JSON.stringify(release, null, 2) + '\n')
writeFileSync(join(stage, 'COMPATIBILITY.md'), compatibilitySummary(release))

const files = walk(stage).filter(path => basename(path) !== 'SHA256SUMS').sort()
writeFileSync(join(stage, 'SHA256SUMS'), files.map(path => `${sha256(path)}  ${relative(stage, path).split(sep).join('/')}`).join('\n') + '\n')
createArchive(stage, archive)
console.log(archive)

function createArchive(stage, archive) {
  if (process.platform !== 'win32') {
    run('tar', ['-czf', archive, '-C', stage, '.'], ROOT)
    return
  }
  // Windows file modes do not survive bsdtar consistently. Normalize inside
  // the WSL filesystem so the uploaded production archive has executable
  // binaries/scripts and non-executable assets/configuration.
  const wslStage = command('wsl.exe', ['-e', 'wslpath', '-a', stage], ROOT).stdout.trim()
  const wslArchive = command('wsl.exe', ['-e', 'wslpath', '-a', archive], ROOT).stdout.trim()
  const script = `set -euo pipefail
stage=$1
archive=$2
temp=$(mktemp -d /tmp/measix-release-XXXXXX)
trap 'rm -rf -- "$temp"' EXIT
cp -a -- "$stage/." "$temp/"
find "$temp" -type d -exec chmod 0755 {} +
find "$temp" -type f -exec chmod 0644 {} +
chmod 0755 "$temp"/bin/* "$temp"/deploy/*.sh
tar -czf "$archive" -C "$temp" .`
  run('wsl.exe', ['-e', 'bash', '-lc', script, 'measix-release', wslStage, wslArchive], ROOT)
}

function migrationIdentity() {
  const dir = join(ROOT, 'backend', 'migrations')
  const files = readdirSync(dir).filter(name => /^\d{6}_.+\.sql$/.test(name)).sort()
  const hash = createHash('sha256')
  files.forEach((name, index) => hash.update(`${String(index + 1).padStart(6, '0')}\0${name}\0${sha256(join(dir, name))}\n`))
  return `sha256:${hash.digest('hex')}`
}

function compatibilityEvidence() {
  const portalContract = JSON.parse(readFileSync(join(PORTAL, 'src', 'api', 'contract.json'), 'utf8'))
  const association = verification.association
  const androidPortal = association ? JSON.parse(readFileSync(join(association.release.base, association.release.record.contracts.directory, 'portal/manifest.json'), 'utf8')) : null
  return {
    baseline: JSON.parse(readFileSync(join(ROOT, 'api', 'protocol-baseline.json'), 'utf8')).baseline,
    clientProtocolVersion: '1',
    snapshotSchemaVersions: snapshotVersions(ROOT).supported,
    snapshotPublicationSchemaVersion: snapshotVersions(ROOT).current,
    androidSnapshotSchemaVersions: association?.release.record.snapshotSchemaVersions ?? null,
    enrollmentFormatVersion: 1,
    portalBridgeVersion: 3,
    portalArtifacts: portalContract.artifacts,
    androidClientContract: association ? `sha256:${sha256(association.release.clientFile)}` : null,
    androidPortalArtifacts: androidPortal?.artifacts ?? null,
    verificationMode: options.mode,
    verifiedAndroid: [],
  }
}

function walk(dir) {
  return readdirSync(dir).flatMap(name => {
    const path = join(dir, name)
    return statSync(path).isDirectory() ? walk(path) : [path]
  })
}
function sha256(path) { return createHash('sha256').update(readFileSync(path)).digest('hex') }
function git(cwd, args) { return command('git', args, cwd).stdout }
function run(name, args, cwd, extraEnv = {}) {
  const result = command(name, args, cwd, extraEnv)
  if (result.stdout) process.stdout.write(result.stdout)
  if (result.stderr) process.stderr.write(result.stderr)
}
function command(name, args, cwd, extraEnv = {}) {
  const result = spawnSync(name, args, { cwd, encoding: 'utf8', env: { ...process.env, ...extraEnv }, shell: process.platform === 'win32' && name === 'pnpm', windowsHide: true, maxBuffer: 32 << 20 })
  if (result.status !== 0) fail(`${name} ${args.join(' ')} failed\n${result.stdout ?? ''}${result.stderr ?? ''}`)
  return result
}
function fail(message) { console.error(message); process.exit(1) }

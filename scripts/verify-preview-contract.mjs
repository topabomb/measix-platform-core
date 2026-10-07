#!/usr/bin/env node
import { createHash } from 'node:crypto'
import { existsSync, readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { isDeepStrictEqual } from 'node:util'
import { releaseAndroidRoot } from './lib/release-paths.mjs'
import { platformContractIdentity, requireIdentity, readJSON, validateAndroidRelease, validateConsumerEvidence, checkedArtifact, lfHash } from './lib/release-contract.mjs'
import { spawnSync } from 'node:child_process'
import { androidSnapshotVersions } from './lib/harness.mjs'

export function androidContractFailures(clientSchema, coreManifest, androidSchema, androidManifest, platformWire) {
  const failures = []
  const canonical = value => Buffer.from(value).toString('utf8').replaceAll('\r\n', '\n')
  const source = canonical(clientSchema)
  const sourceHash = createHash('sha256').update(source).digest('hex')
  if (canonical(androidSchema) !== source) failures.push('Android client contract fixture differs from Core')
  if (coreManifest.sourceHash !== `sha256:${sourceHash}` || coreManifest.generated !== true
      || coreManifest.format !== 'openapi-3.0.3-client-schema-only') failures.push('Core Android manifest identity is invalid')
  if (!isDeepStrictEqual(androidManifest, coreManifest)) failures.push('Android platform manifest differs from Core')
  const header = /^\/\/ Core source SHA256 \(LF\): ([a-f0-9]{64})\r?$/m.exec(platformWire)
  if (header?.[1] !== sourceHash) failures.push('Android PlatformWire source hash is stale')
  return failures
}

export function verifyPreviewContract(root, options = {}) {
  const mode = options.mode ?? 'joint'
  if (!['core', 'joint', 'independent'].includes(mode)) throw new Error('Invalid verification mode')
  const portal = resolve(root, '..', 'measix-enterprise-portal')
  const android = releaseAndroidRoot(root)
  const baselineRaw = readFileSync(join(root, 'api', 'protocol-baseline.json'))
  const baseline = JSON.parse(baselineRaw)
  const identity = platformContractIdentity(baselineRaw)
  const failures = []
  for (const [name, expected] of Object.entries(baseline.documents ?? {})) {
    const actual = `sha256:${createHash('sha256').update(readFileSync(join(root, name))).digest('hex')}`
    if (actual !== expected) failures.push(`${name}: expected ${expected}, actual ${actual}`)
  }

  const clientHash = lfHash(readFileSync(join(root, 'api/client/client-control.openapi.yaml')))
  checkFileEquals(join(root, 'api/generated/android/client-control.openapi.yaml'), join(root, 'api/client/client-control.openapi.yaml'), 'Core Android client export')
  const coreAndroidManifest = readJSON(join(root, 'api/generated/android/manifest.json'))
  if (coreAndroidManifest.sourceHash !== clientHash) failures.push(`Core Android manifest sourceHash: expected ${clientHash}, actual ${coreAndroidManifest.sourceHash}`)
  requireIdentity(coreAndroidManifest, identity, 'Core Client export')
  if (coreAndroidManifest.generated !== true || coreAndroidManifest.format !== 'openapi-3.0.3-client-schema-only') failures.push('Core Client export metadata is invalid')
  requireIdentity(platformContractIdentity(readFileSync(join(root, 'api/generated/android/protocol-baseline.json'))), identity, 'Core baseline export')
  const generatedPortal = readJSON(join(root, 'api/generated/android/portal/manifest.json'))
  requireIdentity(generatedPortal, identity, 'Core Portal export')
  if (generatedPortal.bridgeVersion !== 3 || !generatedPortal.artifacts || !Object.keys(generatedPortal.artifacts).length) failures.push('Core Portal export is incomplete')
  for (const [name, item] of Object.entries(generatedPortal.artifacts ?? {})) {
    if (!item.source?.startsWith('api/')) throw new Error('Invalid Core Portal artifact source')
    checkedArtifact(root, { path: item.source, sha256: 'sha256:' + item.sha256 })
    checkedArtifact(join(root, 'api/generated/android/portal'), { path: name, sha256: 'sha256:' + item.sha256 })
  }

  if (mode !== 'core') {
    const portalManifest = readJSON(join(portal, 'src/api/contract.json'))
    for (const [name, item] of Object.entries(portalManifest.artifacts ?? {})) {
      const prefix = 'measix-platform-core/'
      if (!item.source?.startsWith(prefix)) {
        failures.push(`Portal ${name}: invalid source ${item.source}`)
        continue
      }
      const source = join(root, item.source.slice(prefix.length))
      if (!existsSync(source)) failures.push(`Portal ${name}: source does not exist ${item.source}`)
      else if (`sha256:${item.sha256}` !== hash(source)) failures.push(`Portal ${name}: stale source hash`)
    }

    if (mode === 'joint') {
      const androidPlatform = join(android, 'app/src/test/resources/contracts/platform/client-control.openapi.yaml')
      checkFileEquals(join(android, 'app/src/test/resources/contracts/platform/snapshot-reception-cases.json'),
        join(root, 'api/fixtures/client-integration/snapshot-reception-cases.json'), 'Android snapshot reception cases')
      const platformWire = readFileSync(join(android, 'app/src/main/java/net/weero/measix/pilot/data/enterprise/PlatformWire.kt'), 'utf8')
      failures.push(...androidContractFailures(readFileSync(join(root, 'api/client/client-control.openapi.yaml')), coreAndroidManifest, readFileSync(androidPlatform), readJSON(join(android, 'app/src/test/resources/contracts/platform/manifest.json')), platformWire))

      const corePortalDir = join(root, 'api/generated/android/portal')
      const androidPortalDir = join(android, 'app/src/test/resources/contracts/portal')
      const corePortalManifest = readJSON(join(corePortalDir, 'manifest.json'))
      const androidPortalManifest = readJSON(join(androidPortalDir, 'manifest.json'))
      if (JSON.stringify(androidPortalManifest) !== JSON.stringify(corePortalManifest)) failures.push('Android portal manifest is stale')
      for (const name of Object.keys(corePortalManifest.artifacts ?? {})) {
        checkFileEquals(join(androidPortalDir, name), join(corePortalDir, name), `Android portal fixture ${name}`)
      }
    }
  }
  if (!baseline.baseline || !baseline.policy || Object.keys(baseline.documents ?? {}).length !== 4) failures.push('baseline metadata is incomplete')
  if (failures.length) {
    throw new Error(`Preview protocol baseline mismatch. Review compatibility and update the baseline deliberately:\n${failures.join('\n')}`)
  }
  let association = null
  if (mode === 'independent' || options.androidRelease || options.evidence) {
    if (!options.androidRelease || !options.evidence || !options.version) throw new Error('Verified release requires --android-release, --evidence and --version')
    const release = validateAndroidRelease(resolve(options.androidRelease), { ...identity, clientSourceHash: coreAndroidManifest.sourceHash }, mode)
    if (mode === 'joint' && release.record.sourceCommit !== gitCommit(android)) throw new Error('Joint Android release commit differs from checkout')
    if (mode === 'joint' && !isDeepStrictEqual(release.record.snapshotSchemaVersions, androidSnapshotVersions(android))) throw new Error('Joint Android Snapshot support differs from release record')
    const evidence = validateConsumerEvidence(resolve(options.evidence), release, {
      version: options.version, sourceCommit: gitCommit(root), architectureCommit: gitCommit(resolve(root, '../measix-architecture')),
      portalCommit: gitCommit(portal), baselineHash: identity.baselineHash,
    })
    association = { release, evidence }
  }
  return { identity, baseline: baseline.baseline, association, mode }

  function hash(path) {
    return `sha256:${createHash('sha256').update(readFileSync(path)).digest('hex')}`
  }
  function checkFileEquals(actual, expected, label) {
    if (!existsSync(actual)) failures.push(`${label}: missing ${actual}`)
    else if (hash(actual) !== hash(expected)) failures.push(`${label}: content does not match canonical export`)
  }

}
function gitCommit(root) {
  const result = spawnSync('git', ['rev-parse', 'HEAD'], { cwd: root, encoding: 'utf8', windowsHide: true })
  if (result.status !== 0) throw new Error('Cannot determine fixed source commit')
  return result.stdout.trim()
}

export function parseVerificationArgs(args) {
  const options = {}
  for (let i = 0; i < args.length; i += 2) {
    const key = { '--mode': 'mode', '--version': 'version', '--android-release': 'androidRelease', '--evidence': 'evidence' }[args[i]]
    if (!key || !args[i + 1] || args[i + 1].startsWith('--') || options[key]) throw new Error('Usage: verify-preview-contract.mjs [--mode core|joint|independent] [--version V --android-release RECORD --evidence PROOF]')
    options[key] = args[i + 1]
  }
  if (options.mode === 'core' && (options.evidence || options.androidRelease)) throw new Error('Core-only verification cannot certify Android compatibility')
  return options
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const result = verifyPreviewContract(resolve(dirname(fileURLToPath(import.meta.url)), '..'), parseVerificationArgs(process.argv.slice(2)))
    console.log(`Preview protocol baseline verified (${result.mode}; ${result.association ? 'consumer evidence checked' : 'materials only, no APK compatibility claim'}): ${result.baseline}`)
  } catch (error) { console.error(error.message); process.exitCode = 1 }
}

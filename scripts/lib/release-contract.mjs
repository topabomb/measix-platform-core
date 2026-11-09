import { createHash } from 'node:crypto'
import { existsSync, readFileSync, realpathSync, readdirSync } from 'node:fs'
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path'
import { isDeepStrictEqual } from 'node:util'

export const identityFields = ['platformContractVersion', 'supportedPlatformContractVersions', 'coreBaselineVersion', 'baselineHash']
export const releaseVersionPattern = /^[0-9A-Za-z][0-9A-Za-z._-]{0,63}$/
const hashPattern = /^sha256:[a-f0-9]{64}$/
const commitPattern = /^[a-f0-9]{40,64}$/
const requireValue = (valid, message) => { if (!valid) throw new Error(message) }
export const bytesHash = bytes => 'sha256:' + createHash('sha256').update(bytes).digest('hex')
export const lfHash = bytes => bytesHash(Buffer.from(bytes).toString('utf8').replaceAll('\r\n', '\n'))
export const readJSON = file => JSON.parse(readFileSync(file, 'utf8'))

export function validateContractIdentity(value) {
  requireValue(Number.isSafeInteger(value.platformContractVersion) && value.platformContractVersion > 0, 'Invalid platformContractVersion')
  const set = value.supportedPlatformContractVersions
  requireValue(Array.isArray(set) && set.length > 0 && set.every((v, i) => Number.isSafeInteger(v) && v > 0 && (!i || v > set[i - 1])) && set.includes(value.platformContractVersion), 'Invalid supportedPlatformContractVersions')
  requireValue(typeof value.coreBaselineVersion === 'string' && releaseVersionPattern.test(value.coreBaselineVersion), 'Invalid coreBaselineVersion')
  return value
}

export function platformContractIdentity(raw) {
  const baseline = validateContractIdentity(JSON.parse(raw))
  return { platformContractVersion: baseline.platformContractVersion, supportedPlatformContractVersions: [...baseline.supportedPlatformContractVersions], coreBaselineVersion: baseline.coreBaselineVersion, baselineHash: lfHash(raw) }
}

export function requireIdentity(manifest, expected, label) {
  validateContractIdentity(manifest)
  for (const field of identityFields) requireValue(isDeepStrictEqual(manifest[field], expected[field]), `${label}: ${field} differs from fixed baseline`)
}

export function assertReleaseOutputAvailable(stage, archive) {
  requireValue(!existsSync(stage) && !existsSync(archive), 'Release identity already exists or is occupied; preserve original output')
}

// Resolve every supplied artifact against its record directory, including symlinks.
export function pinnedFile(base, name) {
  requireValue(typeof name === 'string' && name.length > 0 && !isAbsolute(name), 'Pinned artifact path must be relative')
  const root = realpathSync(base)
  const path = resolve(root, name)
  const inside = file => { const rel = relative(root, file); return rel && rel !== '..' && !rel.startsWith('..' + sep) && !isAbsolute(rel) }
  requireValue(inside(path), 'Artifact escapes record directory')
  const file = realpathSync(path)
  requireValue(inside(file), 'Artifact escapes record directory')
  return file
}

export function checkedArtifact(base, item) {
  requireValue(item && hashPattern.test(item.sha256), 'Missing artifact SHA-256')
  const file = pinnedFile(base, item.path)
  requireValue(bytesHash(readFileSync(file)) === item.sha256, `Artifact hash mismatch: ${item.path}`)
  return file
}

export function payloadHash(stage) {
  const entries = []
  function walk(dir) {
    for (const item of readdirSync(dir, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name, 'en'))) {
      const file = join(dir, item.name)
      requireValue(!item.isSymbolicLink(), 'Symlink in release payload')
      if (item.isDirectory()) walk(file)
      else entries.push(relative(stage, file).split(sep).join('/') + '\0' + bytesHash(readFileSync(file)))
    }
  }
  for (const name of ['bin', 'assets', 'deploy']) walk(join(stage, name))
  return bytesHash(entries.sort().join('\n'))
}

export function validateAndroidRelease(file, coreIdentity, mode = 'independent') {
  const record = readJSON(file), base = dirname(resolve(file))
  requireValue(record.formatVersion === 1 && record.product === 'MEASIX Android' && record.sourceDirty === false && commitPattern.test(record.sourceCommit), 'Invalid Android release source identity')
  requireValue(['candidate', 'released'].includes(record.status), 'Invalid Android release status')
  requireValue(typeof record.applicationId === 'string' && /^[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)+$/.test(record.applicationId) && typeof record.versionName === 'string' && releaseVersionPattern.test(record.versionName) && Number.isSafeInteger(record.versionCode) && record.versionCode > 0, 'Invalid Android product version identity')
  if (record.status === 'released') requireValue(record.tag === 'v' + record.versionName, 'Android tag/versionName mismatch')
  validateContractIdentity(record)
  requireValue(record.supportedPlatformContractVersions.includes(coreIdentity.platformContractVersion) && coreIdentity.supportedPlatformContractVersions.includes(record.platformContractVersion), 'Android/Core platform contract is not supported')
  const directory = pinnedFile(base, record.contracts?.directory)
  const clientFile = join(directory, 'platform/client-control.openapi.yaml')
  const clientManifestFile = checkedArtifact(base, record.contracts.platformManifest)
  const portalManifestFile = checkedArtifact(base, record.contracts.portalManifest)
  requireValue(realpathSync(clientManifestFile) === realpathSync(join(directory, 'platform/manifest.json')) && realpathSync(portalManifestFile) === realpathSync(join(directory, 'portal/manifest.json')), 'Android contract manifest path mismatch')
  const original = platformContractIdentity(readFileSync(pinnedFile(directory, 'platform/protocol-baseline.json')))
  const client = readJSON(clientManifestFile), portal = readJSON(portalManifestFile)
  requireIdentity(client, original, 'Android Client manifest')
  requireIdentity(portal, original, 'Android Portal manifest')
  requireValue(record.platformContractVersion === original.platformContractVersion && record.coreBaselineVersion === original.coreBaselineVersion && record.baselineHash === original.baselineHash, 'Android release semantic baseline mismatch')
  requireValue(client.generated === true && client.format === 'openapi-3.0.3-client-schema-only' && client.sourceHash === lfHash(readFileSync(pinnedFile(directory, 'platform/client-control.openapi.yaml'))), 'Android original Client source hash mismatch')
  const wire = readFileSync(checkedArtifact(base, record.contracts.wire), 'utf8')
  requireValue(/^\/\/ Core source SHA256 \(LF\): ([a-f0-9]{64})\r?$/m.exec(wire)?.[1] === client.sourceHash.slice(7), 'Android original PlatformWire source hash mismatch')
  requireValue(portal.bridgeVersion === 3 && Object.keys(portal.artifacts ?? {}).length > 0, 'Android original Portal manifest incomplete')
  for (const [name, artifact] of Object.entries(portal.artifacts)) checkedArtifact(join(directory, 'portal'), { path: name, sha256: 'sha256:' + artifact.sha256 })
  if (mode === 'joint') requireValue(original.baselineHash === coreIdentity.baselineHash && client.sourceHash === coreIdentity.clientSourceHash, 'Joint release baseline differs from Core')
  requireValue(Array.isArray(record.artifacts) && record.artifacts.length > 0, 'Android release has no APK artifacts')
  requireValue(Array.isArray(record.snapshotSchemaVersions) && record.snapshotSchemaVersions.length > 0 && record.snapshotSchemaVersions.every((v, i, a) => Number.isSafeInteger(v) && v > 0 && (!i || v > a[i - 1])), 'Invalid Android Snapshot support identity')
  const variants = new Set()
  for (const apk of record.artifacts) {
    requireValue(['debug', 'release'].includes(apk.variant) && typeof apk.abi === 'string' && /^[a-z0-9_-]+$/.test(apk.abi) && hashPattern.test(apk.signingCertificateSha256), 'Invalid APK variant/signing identity')
    if (record.status === 'released') requireValue(apk.variant === 'release', 'Debug APK is not a formal Android release')
    requireValue(!variants.has(apk.abi + '/' + apk.variant), 'Duplicate APK variant')
    variants.add(apk.abi + '/' + apk.variant)
    checkedArtifact(base, apk)
  }
  return { record, base, file: resolve(file), recordHash: bytesHash(readFileSync(file)), clientFile }
}

export function validateConsumerEvidence(file, android, expectedCore) {
  // A Core build can take time; cached preflight identity must not hide changed inputs.
  const current = validateAndroidRelease(android.file, android.record)
  requireValue(current.recordHash === android.recordHash, 'Android record changed after preflight')
  const proof = readJSON(file), base = dirname(resolve(file))
  requireValue(proof.formatVersion === 1 && proof.result === 'PASS' && proof.sourceDirty === false && proof.suite === 'android-core-consumer', 'Missing successful clean Android consumer evidence')
  for (const [key, value] of Object.entries(expectedCore)) requireValue(value !== undefined && proof.core?.[key] === value, `Consumer evidence Core ${key} mismatch`)
  requireValue(hashPattern.test(proof.core?.buildHash), 'Consumer evidence has no Core payload build hash')
  requireValue(proof.android?.releaseHash === android.recordHash && proof.android?.sourceCommit === android.record.sourceCommit, 'Consumer evidence Android release/source mismatch')
  const artifact = android.record.artifacts.find(apk => apk.sha256 === proof.android?.apkSha256)
  requireValue(artifact, 'Consumer evidence APK mismatch')
  requireValue(Array.isArray(proof.checks) && proof.checks.length === 4 && ['identity', 'snapshot', 'runtime', 'portal'].every(id => proof.checks.filter(c => c.id === id).length === 1), 'Consumer evidence required checks incomplete')
  for (const check of proof.checks) {
    requireValue(check.exitCode === 0 && typeof check.command === 'string' && check.command.trim().length > 0, 'Consumer check command/exit failed')
    const xml = readFileSync(checkedArtifact(base, check.report), 'utf8')
    checkedArtifact(base, check.log)
    const body = xml.replace(/<!--[\s\S]*?-->|<!\[CDATA\[[\s\S]*?\]\]>/g, '').replace(/<\?xml[^>]*\?>/, '').trim()
    const suites = [...body.matchAll(/<testsuite\b([^>]*)>/g)]
    requireValue(!/<!DOCTYPE|<!ENTITY/i.test(body) && /^<testsuites?\b/.test(body) && /<\/testsuites?>$/.test(body) && /<testcase\b/.test(body) && suites.length > 0 && !/<(?:failure|error|skipped)\b/.test(body), 'Consumer JUnit failure/error/skip or malformed report')
    let total = 0
    for (const [, attrs] of suites) {
      const counters = Object.fromEntries([...attrs.matchAll(/\b(tests|failures|errors|skipped)=["'](\d+)["']/g)].map(([, key, count]) => [key, count]))
      for (const key of ['tests', 'failures', 'errors', 'skipped']) {
        const count = counters[key]
        requireValue(count !== undefined && (key === 'tests' || Number(count) === 0), 'Consumer JUnit counters incomplete or failed')
        if (key === 'tests') total += Number(count)
      }
    }
    requireValue(total > 0, 'Consumer JUnit has no executed tests')
  }
  return { proof, base, evidenceHash: bytesHash(readFileSync(file)), artifact }
}

export function compatibilitySummary(release) {
  const rows = release.compatibility.verifiedAndroid ?? []
  return `# Core / Android 版本对应\n\nCore：${release.version}\n\n` +
    (rows.length ? '| Android（versionCode） | Core | 结果 | Android 状态 | APK SHA-256 |\n| --- | --- | --- | --- | --- |\n' + rows.map(r => `| ${r.versionName}（${r.versionCode}） | ${release.version} | 已验证 | ${r.status} | ${r.apkSha256} |`).join('\n') + '\n' : '本包未附自动验证结果，不表示已知不兼容。后续结果查看引用本包摘要的发行记录；没有实际测试不得声明支持。\n') +
    `\n内部追溯：合同 ${release.platformContractVersion}；基准 Core ${release.coreBaselineVersion}；打包状态 ${release.publicationStatus}。完整身份见 release.json。\n`
}

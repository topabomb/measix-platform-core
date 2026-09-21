#!/usr/bin/env node
import { createHash } from 'node:crypto'
import { existsSync, readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const portal = resolve(root, '..', 'measix-enterprise-portal')
const android = resolve(root, '..', '..', 'rikkahub_mcp')
const baseline = JSON.parse(readFileSync(join(root, 'api', 'protocol-baseline.json'), 'utf8'))
const failures = []
for (const [name, expected] of Object.entries(baseline.documents ?? {})) {
  const actual = `sha256:${createHash('sha256').update(readFileSync(join(root, name))).digest('hex')}`
  if (actual !== expected) failures.push(`${name}: expected ${expected}, actual ${actual}`)
}

const clientHash = hash(join(root, 'api/client/client-control.openapi.yaml'))
checkFileEquals(join(root, 'api/generated/android/client-control.openapi.yaml'), join(root, 'api/client/client-control.openapi.yaml'), 'Core Android client export')
const coreAndroidManifest = readJSON(join(root, 'api/generated/android/manifest.json'))
if (coreAndroidManifest.sourceHash !== clientHash) failures.push(`Core Android manifest sourceHash: expected ${clientHash}, actual ${coreAndroidManifest.sourceHash}`)

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

const androidPlatform = join(android, 'app/src/test/resources/contracts/platform/client-control.openapi.yaml')
checkFileEquals(androidPlatform, join(root, 'api/generated/android/client-control.openapi.yaml'), 'Android client contract fixture')
const platformWire = readFileSync(join(android, 'app/src/main/java/net/weero/measix/pilot/data/enterprise/PlatformWire.kt'), 'utf8')
if (!platformWire.includes(`Source SHA256: ${clientHash.slice('sha256:'.length)}`)) failures.push('Android PlatformWire source hash is stale')

const corePortalDir = join(root, 'api/generated/android/portal')
const androidPortalDir = join(android, 'app/src/test/resources/contracts/portal')
const corePortalManifest = readJSON(join(corePortalDir, 'manifest.json'))
const androidPortalManifest = readJSON(join(androidPortalDir, 'manifest.json'))
if (JSON.stringify(androidPortalManifest) !== JSON.stringify(corePortalManifest)) failures.push('Android portal manifest is stale')
for (const name of Object.keys(corePortalManifest.artifacts ?? {})) {
  checkFileEquals(join(androidPortalDir, name), join(corePortalDir, name), `Android portal fixture ${name}`)
}
if (!baseline.baseline || !baseline.policy || Object.keys(baseline.documents ?? {}).length !== 4) failures.push('baseline metadata is incomplete')
if (failures.length) {
  console.error(`Preview protocol baseline mismatch. Review compatibility and update the baseline deliberately:\n${failures.join('\n')}`)
  process.exit(1)
}
console.log(`Preview protocol baseline verified: ${baseline.baseline}`)

function hash(path) {
  return `sha256:${createHash('sha256').update(readFileSync(path)).digest('hex')}`
}
function readJSON(path) { return JSON.parse(readFileSync(path, 'utf8')) }
function checkFileEquals(actual, expected, label) {
  if (!existsSync(actual)) failures.push(`${label}: missing ${actual}`)
  else if (hash(actual) !== hash(expected)) failures.push(`${label}: content does not match canonical export`)
}

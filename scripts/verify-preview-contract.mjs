#!/usr/bin/env node
import { createHash } from 'node:crypto'
import { existsSync, readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { isDeepStrictEqual } from 'node:util'

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

function main() {
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

}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main()

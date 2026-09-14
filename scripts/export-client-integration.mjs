import { createHash } from 'node:crypto'
import { cpSync, lstatSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs'
import { dirname, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = resolve(import.meta.dirname, '..')
const PARENT = resolve(ROOT, '..')
const OUTPUT = resolve(ROOT, 'api/generated/android/integration')
const INPUTS = [
  'measix-platform-core/api/generated/android/client-control.openapi.yaml',
  'measix-platform-core/api/generated/android/portal',
  'measix-platform-core/api/fixtures/client-integration',
  'measix-platform-core/api/fixtures/enrollment',
  'measix-platform-core/api/fixtures/portal',
  'measix-platform-core/api/fixtures/problem',
  'measix-platform-core/docs/android-platform-integration.md',
  'measix-architecture/docs',
]
const hash = bytes => createHash('sha256').update(bytes).digest('hex')
function files(path) {
  if (lstatSync(path).isSymbolicLink()) throw new Error('Symlink in integration material')
  return lstatSync(path).isDirectory()
    ? readdirSync(path).sort().flatMap(name => files(resolve(path, name))) : [path]
}
function safePath(name) {
  if (!name || name.includes('\\') || name.includes(':') || name.split('/').some(part => !part || part === '..' || part === '.')) throw new Error('Unsafe artifact path')
}
export function verifyIntegration(directory) {
  const root = resolve(directory)
  // Enumerate and reject links before following any manifest paths.
  const actual = files(root).map(path => relative(root, path).replaceAll('\\', '/')).filter(name => name !== 'manifest.json').sort()
  const manifest = JSON.parse(readFileSync(resolve(root, 'manifest.json'), 'utf8'))
  if (manifest.formatVersion !== 1 || manifest.snapshotSchemaVersion !== 4 || manifest.bridgeVersion !== 3 || manifest.localReadVersion !== 2) throw new Error('Wrong integration profile')
  const names = Object.keys(manifest.artifacts).sort()
  for (const name of names) {
    safePath(name)
    if (hash(readFileSync(resolve(root, name))) !== manifest.artifacts[name]) throw new Error('Artifact digest mismatch: ' + name)
  }
  if (JSON.stringify(names) !== JSON.stringify(actual)) throw new Error('Unlisted or missing integration files')
  if (hash(names.map(name => name + '\0' + manifest.artifacts[name]).join('\n')) !== manifest.sourceHash) throw new Error('Integration source hash mismatch')
  return manifest
}
export function exportIntegration() {

  // Fixed generated output owned by this script; never delete a caller-supplied path.
  if (relative(ROOT, OUTPUT).replaceAll('\\', '/') !== 'api/generated/android/integration') throw new Error('Unsafe output')
  rmSync(OUTPUT, { recursive: true, force: true })
  mkdirSync(OUTPUT, { recursive: true })
  for (const input of INPUTS) {
    for (const source of files(resolve(PARENT, input))) {
      const target = resolve(OUTPUT, relative(PARENT, source))
      mkdirSync(dirname(target), { recursive: true })
      cpSync(source, target)
    }
  }
  cpSync(fileURLToPath(import.meta.url), resolve(OUTPUT, 'verify.mjs'))
  writeFileSync(resolve(OUTPUT, 'README.md'), '# Android platform integration materials\n\nRead [the integration guide](measix-platform-core/docs/android-platform-integration.md). Run `node verify.mjs --verify .` from this directory. All SHA-256 values cover exact file bytes; no sibling repository is needed. Synthetic credentials are not usable. This is contract material, not Android device acceptance.\n')
  const artifacts = Object.fromEntries(files(OUTPUT).map(path => [relative(OUTPUT, path).replaceAll('\\', '/'), hash(readFileSync(path))]).sort(([a], [b]) => a.localeCompare(b, 'en')))
  const sourceHash = hash(Object.keys(artifacts).sort().map(name => name + '\0' + artifacts[name]).join('\n'))
  writeFileSync(resolve(OUTPUT, 'manifest.json'), JSON.stringify({ formatVersion: 1, snapshotSchemaVersion: 4, bridgeVersion: 3, localReadVersion: 2, sourceHash, artifacts }, null, 2) + '\n')
  return verifyIntegration(OUTPUT)
}
export function verifyIntegrationSources(directory = OUTPUT, sourceRoot = PARENT) {
  const manifest = verifyIntegration(directory)
  const expected = INPUTS.flatMap(input => files(resolve(sourceRoot, input))).map(path => relative(sourceRoot, path).replaceAll('\\', '/')).sort()
  const exported = Object.keys(manifest.artifacts).filter(name => name !== 'README.md' && name !== 'verify.mjs').sort()
  if (JSON.stringify(expected) !== JSON.stringify(exported)) throw new Error('Stale integration source inventory; regenerate the export')
  for (const [name, digest] of Object.entries(manifest.artifacts)) {
    if (name === 'README.md') continue // generated text is bound to verify.mjs
    const source = name === 'verify.mjs' ? fileURLToPath(import.meta.url) : resolve(sourceRoot, name)
    if (hash(readFileSync(source)) !== digest) throw new Error('Stale integration source: ' + name)
  }
  return manifest
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const manifest = process.argv[2] === '--verify' ? verifyIntegration(process.argv[3] ?? '.') : exportIntegration()
  console.log(JSON.stringify({ files: Object.keys(manifest.artifacts).length, sourceHash: manifest.sourceHash }))
}

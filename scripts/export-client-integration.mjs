import { cpSync, lstatSync, mkdirSync, readdirSync, rmSync, writeFileSync } from 'node:fs'
import { dirname, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = resolve(import.meta.dirname, '..')
const PARENT = resolve(ROOT, '..')
const OUTPUT = resolve(ROOT, 'api/generated/android/integration')

// Only materials the client actually consumes: the executable control contract,
// the Portal bridge contract, the shared fixtures, the 428 problem sample and
// the handoff guide. The architecture documents stay authoritative in their own
// repository and are referenced by name; vendoring them here would duplicate
// authority and force this export to track a second repository. `problem` is a
// single file because the other problem fixture belongs to the Admin API, which
// no client calls.
const INPUTS = [
  'measix-platform-core/api/generated/android/client-control.openapi.yaml',
  'measix-platform-core/api/generated/android/portal',
  'measix-platform-core/api/fixtures/client-integration',
  'measix-platform-core/api/fixtures/enrollment',
  'measix-platform-core/api/fixtures/portal',
  'measix-platform-core/api/fixtures/problem/managed-snapshot-required.json',
  'measix-platform-core/docs/android-platform-integration.md',
]

function files(path) {
  if (lstatSync(path).isSymbolicLink()) throw new Error('Symlink in integration material')
  return lstatSync(path).isDirectory()
    ? readdirSync(path).sort().flatMap(name => files(resolve(path, name))) : [path]
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
  writeFileSync(resolve(OUTPUT, 'README.md'), '# Android platform integration materials\n\nStart with [the integration guide](measix-platform-core/docs/android-platform-integration.md). This directory is a copy of contract material owned by `measix-platform-core`; edit the sources, not this directory.\n')
  return OUTPUT
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const output = exportIntegration()
  console.log('exported ' + relative(ROOT, output).replaceAll('\\', '/'))
}

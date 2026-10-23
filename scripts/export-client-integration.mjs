import { cpSync, lstatSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
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
  'measix-platform-core/api/generated/android/manifest.json',
  'measix-platform-core/api/generated/android/protocol-baseline.json',
  'measix-platform-core/api/generated/android/portal',
  'measix-platform-core/api/fixtures/client-integration',
  'measix-platform-core/api/fixtures/enrollment',
  'measix-platform-core/api/fixtures/portal',
  'measix-platform-core/api/fixtures/workspace/projection-unprovisioned.json',
  'measix-platform-core/api/fixtures/workspace/projection-files-only.json',
  'measix-platform-core/api/fixtures/problem/managed-snapshot-required.json',
  'measix-platform-core/docs/android-platform-integration.md',
  'measix-platform-core/docs/direct-mcp-tool-governance.md',
]

function files(path) {
  if (lstatSync(path).isSymbolicLink()) throw new Error('Symlink in integration material')
  return lstatSync(path).isDirectory()
    ? readdirSync(path).sort().flatMap(name => files(resolve(path, name))) : [path]
}

export function exportIntegration() {
  const sources = INPUTS.flatMap(input => files(resolve(PARENT, input)))
  const included = new Set(sources)
  // Fixed generated output owned by this script; never delete a caller-supplied path.
  if (relative(ROOT, OUTPUT).replaceAll('\\', '/') !== 'api/generated/android/integration') throw new Error('Unsafe output')
  rmSync(OUTPUT, { recursive: true, force: true })
  mkdirSync(OUTPUT, { recursive: true })
  for (const source of sources) {
    const target = resolve(OUTPUT, relative(PARENT, source))
    mkdirSync(dirname(target), { recursive: true })
    if (source.endsWith('.md')) {
      // Keep bundled links clickable; identify source-only references without
      // broken links or copying another repository's documentation authority.
      const text = readFileSync(source, 'utf8').replace(/\[([^\]]+)\]\(([^)]+)\)/g, (link, label, ref) => {
        if (/^(?:[a-z][a-z0-9+.-]*:|#)/i.test(ref)) return link
        const [name, anchor] = ref.split('#')
        const referenced = resolve(dirname(source), name)
        if (included.has(referenced)) return link
        const original = relative(PARENT, referenced).replaceAll('\\', '/') + (anchor ? '#' + anchor : '')
        return `${label}（源仓库：\`${original}\`）`
      })
      writeFileSync(target, text)
    } else cpSync(source, target)
  }
  writeFileSync(resolve(OUTPUT, 'README.md'), '# Android platform integration materials\n\nStart with [the integration guide](measix-platform-core/docs/android-platform-integration.md). This directory is a copy of contract material owned by `measix-platform-core`; edit the sources, not this directory. References to architecture, tests and release procedures belong to the source repositories and are not vendored here.\n')
  return OUTPUT
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const output = exportIntegration()
  console.log('exported ' + relative(ROOT, output).replaceAll('\\', '/'))
}

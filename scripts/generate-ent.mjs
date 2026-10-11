import { cpSync, existsSync, mkdtempSync, readdirSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { basename, join, resolve, sep } from 'node:path'
import { spawnSync } from 'node:child_process'

// Ent writes unformatted files before goimports rewrites them. On Windows,
// indexing the live checkout can hold a mapped section during that second
// write. Generate under the same module identity in an isolated directory,
// then copy only the completed generated tree into the checkout.
const root = resolve(import.meta.dirname, '..')
const backend = join(root, 'backend')
const tempRoot = resolve(tmpdir())
const staging = mkdtempSync(join(tempRoot, 'measix-ent-'))
try {
  for (const file of ['go.mod', 'go.sum']) cpSync(join(backend, file), join(staging, file))
  cpSync(join(backend, 'ent/schema'), join(staging, 'ent/schema'), { recursive: true })
  const result = spawnSync('go', ['run', 'entgo.io/ent/cmd/ent@v0.14.6', 'generate', './schema'], {
    cwd: join(staging, 'ent'), stdio: 'inherit', windowsHide: true,
  })
  if (result.error) throw result.error
  if (result.status !== 0) throw new Error(`Ent generation failed (${result.status})`)
  for (const entry of readdirSync(join(staging, 'ent'))) {
    if (entry === 'schema' || entry === 'generate.go') continue
    cpSync(join(staging, 'ent', entry), join(backend, 'ent', entry), { recursive: true, force: true })
  }
} finally {
  const target = resolve(staging)
  if (!target.startsWith(tempRoot + sep) || !basename(target).startsWith('measix-ent-')) {
    throw new Error('Refusing cleanup outside the allocated Ent staging directory')
  }
  if (existsSync(target)) rmSync(target, { recursive: true, force: true })
}

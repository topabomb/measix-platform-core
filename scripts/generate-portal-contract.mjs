import { createHash } from 'node:crypto'
import { mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { execFileSync } from 'node:child_process'
const root = resolve(import.meta.dirname, '..')
execFileSync('go', ['run', './cmd/generate-portal-feed-schema'], { cwd: resolve(root, 'backend'), stdio: 'inherit' })
const output = resolve(root, 'api/generated/android/portal')
rmSync(output, { recursive: true, force: true })
mkdirSync(output, { recursive: true })
const sources = ['portal/portal-contract.openapi.json', 'portal/client-feed.schemas.json', 'fixtures/portal/native-vectors.json', 'fixtures/portal/feed-vectors.json', 'fixtures/enrollment/platform-v1.json', 'fixtures/enrollment/cases.json']
const artifacts = {}
for (const source of sources) {
  const data = readFileSync(resolve(root, 'api', source))
  const name = source.split('/').at(-1)
  writeFileSync(resolve(output, name), data)
  artifacts[name] = { source: 'api/' + source, sha256: createHash('sha256').update(data).digest('hex') }
}
writeFileSync(resolve(output, 'manifest.json'), JSON.stringify({ bridgeVersion: 3, artifacts }, null, 2) + '\n')

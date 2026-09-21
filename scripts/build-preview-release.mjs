#!/usr/bin/env node
import { createHash } from 'node:crypto'
import { chmodSync, cpSync, existsSync, mkdirSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs'
import { basename, dirname, join, relative, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { spawnSync } from 'node:child_process'

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const PORTAL = resolve(ROOT, '..', 'measix-enterprise-portal')
const version = process.argv[2]
if (!version || !/^[0-9A-Za-z][0-9A-Za-z._-]{0,63}$/.test(version)) fail('Usage: node scripts/build-preview-release.mjs <version>')
if (!existsSync(join(PORTAL, 'package.json'))) fail(`Portal repository not found: ${PORTAL}`)
if (git(ROOT, ['status', '--porcelain']).trim()) fail('Core worktree must be clean before building a release')
if (git(PORTAL, ['status', '--porcelain']).trim()) fail('Portal worktree must be clean before building a release')
run('node', ['scripts/verify-preview-contract.mjs'], ROOT)
run('node', ['scripts/checks.mjs', 'generate'], ROOT)
if (git(ROOT, ['status', '--porcelain']).trim()) fail('Generated contracts or dependencies drift from committed sources')

const packageName = `measix-core-${version}-linux-arm64`
const outputDir = join(ROOT, '.artifacts', 'releases')
const stage = join(outputDir, packageName)
rmSync(stage, { recursive: true, force: true })
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
  target: { os: 'linux', arch: 'arm64', platform: 'NVIDIA DGX Spark' },
  builtAt: new Date().toISOString(),
  source: { coreCommit: git(ROOT, ['rev-parse', 'HEAD']).trim(), portalCommit: git(PORTAL, ['rev-parse', 'HEAD']).trim() },
  protocols: Object.fromEntries(protocolFiles.map(path => [path, `sha256:${sha256(join(ROOT, path))}`])),
  schemaMigrationIdentity: migrationIdentity(),
}
writeFileSync(join(stage, 'release.json'), JSON.stringify(release, null, 2) + '\n')

const files = walk(stage).filter(path => basename(path) !== 'SHA256SUMS').sort()
writeFileSync(join(stage, 'SHA256SUMS'), files.map(path => `${sha256(path)}  ${relative(stage, path).split(sep).join('/')}`).join('\n') + '\n')
const archive = join(outputDir, `${packageName}.tar.gz`)
rmSync(archive, { force: true })
run('tar', ['-czf', archive, '-C', stage, '.'], ROOT)
console.log(archive)

function migrationIdentity() {
  const dir = join(ROOT, 'backend', 'migrations')
  const files = readdirSync(dir).filter(name => /^\d{6}_.+\.sql$/.test(name)).sort()
  const hash = createHash('sha256')
  files.forEach((name, index) => hash.update(`${String(index + 1).padStart(6, '0')}\0${name}\0${sha256(join(dir, name))}\n`))
  return `sha256:${hash.digest('hex')}`
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

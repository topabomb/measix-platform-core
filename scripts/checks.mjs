#!/usr/bin/env node
import { createHash } from 'node:crypto'
import { mkdirSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { spawnSync } from 'node:child_process'
import { gitCommit, gitDirty, writeMetaJson } from './lib/harness.mjs'

const ROOT = resolve(import.meta.dirname, '..')
const GENERATED = ['backend/go.mod', 'backend/go.sum', 'backend/ent', 'backend/internal/wire', 'backend/migrations/atlas.sum', 'api/generated/android', 'console/pnpm-lock.yaml', 'console/src/api/generated.ts']
export function commandResult(result) {
  const output = String(result.stdout ?? '') + String(result.stderr ?? '') + (result.error?.message ?? '')
  const exitCode = Number.isInteger(result.status) ? result.status : 1
  return { status: exitCode === 0 ? 'PASS' : 'FAIL', exitCode, output, outputHash: 'sha256:' + createHash('sha256').update(output).digest('hex') }
}
export function requireSuccess(result) {
  if (result.status !== 'PASS') throw new Error(result.output || 'Command failed')
  return result
}
function run(command, args, cwd = ROOT) {
  return commandResult(spawnSync(command, args, { cwd, encoding: 'utf8', maxBuffer: 32 * 1024 * 1024, shell: command === 'pnpm' && process.platform === 'win32', windowsHide: true }))
}
function checked(command, args, cwd) {
  const result = requireSuccess(run(command, args, cwd))
  if (result.output) process.stdout.write(result.output)
  return result
}
export function generate() {
  const backend = join(ROOT, 'backend')
  for (const [config, source] of [['admin', 'admin/admin'], ['client', 'client/client-control'], ['relay', 'internal/relay-control'], ['usage', 'internal/usage-ingest']]) {
    checked('go', ['run', 'github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0', '-config', 'api/codegen/' + config + '.yaml', 'api/' + source + '.openapi.yaml'])
  }
  checked('go', ['run', './cmd/generate-android-wire', '../api/client/client-control.openapi.yaml', '../api/generated/android/client-control.openapi.yaml', '../api/generated/android/manifest.json'], backend)
  checked('go', ['generate', './ent'], backend)
  checked('go', ['mod', 'tidy'], backend)
  checked('pnpm', ['install', '--frozen-lockfile'], join(ROOT, 'console'))
  checked('pnpm', ['generate:api'], join(ROOT, 'console'))
  checked('go', ['run', './cmd/migration-checksum'], backend)
}
function formatCheck() {
  const files = requireSuccess(run('git', ['ls-files', '-z', '--cached', '--others', '--exclude-standard', '--', 'backend'])).output.split('\0').filter(f => f.endsWith('.go'))
  let output = ''
  for (let i = 0; i < files.length; i += 64) output += requireSuccess(run('gofmt', ['-l', ...files.slice(i, i + 64)])).output
  return commandResult({ status: output.trim() ? 1 : 0, stdout: output })
}
function driftCheck() {
  const result = run('git', ['status', '--porcelain', '--', ...GENERATED])
  if (result.status === 'FAIL') return result
  return commandResult({ status: result.output.trim() ? 1 : 0, stdout: result.output })
}
function collectStatic() {
  const startedAt = new Date().toISOString()
  const safely = fn => { try { return fn() } catch (error) { return commandResult({ status: 1, stderr: error.message }) } }
  const codegen = safely(() => { generate(); return driftCheck() })
  const artifact = {
    gofmt: safely(formatCheck), goVet: run('go', ['vet', './...'], join(ROOT, 'backend')), codegenDrift: codegen,
    commit: gitCommit(ROOT), workingTreeDirty: gitDirty(ROOT), startedAt, collectedAt: new Date().toISOString(),
  }
  const failed = [artifact.gofmt, artifact.goVet, artifact.codegenDrift].some(check => check.status !== 'PASS')
  const dir = join(ROOT, '.artifacts')
  mkdirSync(dir, { recursive: true })
  writeFileSync(join(dir, 'static-contract.json'), JSON.stringify(artifact, null, 2) + '\n')
  writeMetaJson(dir, 'static-contract.json', ROOT, join(ROOT, '..', 'measix-architecture'), 'node scripts/checks.mjs static', failed ? 1 : 0)
  for (const [name, value] of Object.entries(artifact)) if (value?.status) console.log(name + ': ' + value.status)
  if (failed) throw new Error('Static checks failed; see .artifacts/static-contract.json')
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    switch (process.argv[2]) {
      case 'generate': generate(); break
      case 'fmt': requireSuccess(formatCheck()); break
      case 'drift': requireSuccess(driftCheck()); break
      case 'static': collectStatic(); break
      default: throw new Error('Usage: node scripts/checks.mjs generate|fmt|drift|static')
    }
  } catch (error) { console.error(error.message); process.exitCode = 1 }
}

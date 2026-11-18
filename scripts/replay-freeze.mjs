#!/usr/bin/env node
/** Rebuild pinned source in independent checkouts and replay the required S0.1 path. */
import { createHash } from 'node:crypto'
import { existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { execFileSync, spawnSync } from 'node:child_process'
import { sourceFacts, validateCandidate, validatePins } from './freeze-manifest.mjs'
import { gitDirty } from './lib/harness.mjs'

const ROOT = resolve(import.meta.dirname, '..')
const hash = bytes => 'sha256:' + createHash('sha256').update(bytes).digest('hex')
function git(cwd, args) {
  return execFileSync('git', args, { cwd, encoding: 'utf8', windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] }).trim()
}

export function checkoutPinnedSources({ core, architecture, manifest, temporaryRoot }) {
  for (const field of ['platformCoreCommit', 'architectureCommit']) {
    if (!/^[a-f0-9]{40}$/.test(manifest[field] ?? '')) throw new Error(`${field} requires a full commit SHA`)
  }
  // Keep the replay workspace inside .artifacts: freeze-manifest reads its
  // per-stage logs back when finalizing, so it must survive process exit and
  // must not sit in a directory the OS is free to purge.
  const root = temporaryRoot ? resolve(temporaryRoot) : join(ROOT, '.artifacts', 'replay')
  mkdirSync(root, { recursive: true })
  const workspace = mkdtempSync(join(root, 'measix-clean-source-'))
  const result = { workspace, core: join(workspace, 'measix-platform-core'), architecture: join(workspace, 'measix-architecture') }
  for (const [source, destination, commit] of [[core, result.core, manifest.platformCoreCommit], [architecture, result.architecture, manifest.architectureCommit]]) {
    // --no-local avoids shared objects/hardlinks and copies neither ignored files nor worktree edits.
    git(workspace, ['clone', '--quiet', '--no-local', '--no-checkout', '--', resolve(source), destination])
    git(destination, ['checkout', '--quiet', '--detach', commit])
    if (git(destination, ['rev-parse', 'HEAD']) !== commit || gitDirty(destination)) throw new Error('Pinned checkout is not exact and clean')
  }
  return result
}

export function runReplayCommands(stages, logs, results) {
  mkdirSync(logs, { recursive: true })
  for (const stage of stages) {
    const startedAt = new Date().toISOString()
    console.log(`[replay] ${stage.id}`)
    const execution = spawnSync(stage.command, stage.args, {
      cwd: stage.cwd, encoding: 'utf8', windowsHide: true,
      shell: process.platform === 'win32' && stage.command === 'pnpm',
      timeout: stage.timeout ?? 900_000, maxBuffer: 64 * 1024 * 1024,
    })
    const output = `${execution.stdout ?? ''}${execution.stderr ?? ''}${execution.error?.message ?? ''}`
    const exitCode = execution.status ?? 1
    writeFileSync(join(logs, `${stage.id}.log`), output, { flag: 'wx' })
    results.push({ id: stage.id, command: stage.command, args: stage.args, startedAt, completedAt: new Date().toISOString(), exitCode, outputHash: hash(output) })
    if (exitCode !== 0) throw new Error(`${stage.id} failed with exit ${exitCode}; see ${join(logs, `${stage.id}.log`)}`)
  }
}

async function main() {
  const args = process.argv.slice(2)
  if (args.length !== 2 || args[0] !== '--manifest') throw new Error('Usage: node scripts/replay-freeze.mjs --manifest <candidate.json>')
  const candidateBytes = readFileSync(resolve(args[1]))
  const manifest = JSON.parse(candidateBytes.toString('utf8'))
  const errors = validateCandidate(manifest, { allowPendingReplay: true })
  if (errors.length) throw new Error(errors.join('\n'))
  const output = join(ROOT, '.artifacts', 'replay-artifact.json')
  if (existsSync(output)) throw new Error('Replay artifact already exists; preserve the previous candidate evidence before running a new composition')
  const checkout = checkoutPinnedSources({ core: ROOT, architecture: join(ROOT, '..', 'measix-architecture'), manifest })
  console.log(`[replay] independent workspace: ${checkout.workspace}`)
  const report = {
    status: 'FAIL', replayKind: 'CLEAN_SOURCE_AND_RUNTIME', candidateManifestHash: hash(candidateBytes),
    platformCoreCommit: manifest.platformCoreCommit, architectureCommit: manifest.architectureCommit,
    workspace: checkout.workspace, startedAt: new Date().toISOString(), stages: [],
  }
  const logs = join(checkout.workspace, 'evidence')
  const core = checkout.core, backend = join(core, 'backend'), consoleDir = join(core, 'console')
  try {
    runReplayCommands([
      { id: 'generate', command: process.execPath, args: ['scripts/checks.mjs', 'generate'], cwd: core },
      { id: 'format', command: process.execPath, args: ['scripts/checks.mjs', 'fmt'], cwd: core },
      { id: 'typecheck', command: 'pnpm', args: ['typecheck'], cwd: consoleDir },
      { id: 'admin-build', command: 'pnpm', args: ['build'], cwd: consoleDir },
    ], logs, report.stages)
    if (gitDirty(core) || gitDirty(checkout.architecture)) throw new Error('Regeneration changed pinned source or generated contracts')
    const facts = sourceFacts(core, checkout.architecture)
    const pinErrors = validatePins(manifest, facts, undefined, true)
    if (pinErrors.length) throw new Error(`Rebuilt candidate differs: ${pinErrors.join('; ')}`)
    report.rebuiltFacts = facts
    runReplayCommands([
      { id: 'contract', command: 'go', args: ['test', './internal/contract', '-count=1'], cwd: backend },
      { id: 'go-vet', command: 'go', args: ['vet', './...'], cwd: backend },
      { id: 'backend', command: 'go', args: ['test', './...', '-count=1'], cwd: backend },
      { id: 'smoke', command: 'go', args: ['test', '-tags=smoke', './test/system/scenarios/', '-count=1', '-timeout', '15m'], cwd: backend },
      { id: 'system', command: 'go', args: ['test', './test/system/adapter/', './test/system/client/', '-count=1', '-timeout', '5m'], cwd: backend },
      { id: 'candidate', command: 'go', args: ['test', '-tags=candidate', './test/system/scenarios/', '-count=1', '-timeout', '15m'], cwd: backend },
      { id: 'console', command: 'pnpm', args: ['test'], cwd: consoleDir },
      { id: 'browser', command: process.execPath, args: ['scripts/e2e-harness.mjs'], cwd: core },
    ], logs, report.stages)
    if (gitDirty(core) || gitDirty(checkout.architecture)) throw new Error('Replay modified pinned source')
    report.status = 'PASS'
  } catch (error) {
    report.error = error.message
    throw error
  } finally {
    report.completedAt = new Date().toISOString()
    mkdirSync(join(ROOT, '.artifacts'), { recursive: true })
    writeFileSync(output, JSON.stringify(report, null, 2) + '\n', { flag: 'wx' })
  }
  console.log(`[replay] PASS; evidence: ${output}. Candidate is unchanged; final validation remains required.`)
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch(error => { console.error(error.message); process.exitCode = 1 })
}

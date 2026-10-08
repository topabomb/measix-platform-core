import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, existsSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { execFileSync } from 'node:child_process'
import { checkoutPinnedSources, runReplayCommands } from './replay-freeze.mjs'

function git(cwd, ...args) { return execFileSync('git', args, { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'], windowsHide: true }).trim() }
function repository(root, name) {
  const path = join(root, name)
  mkdirSync(path)
  git(path, 'init', '-q')
  writeFileSync(join(path, 'source.txt'), 'pinned source\n')
  writeFileSync(join(path, '.gitignore'), 'private.key\n')
  git(path, 'add', 'source.txt', '.gitignore')
  git(path, '-c', 'user.name=Replay test', '-c', 'user.email=replay@example.invalid', 'commit', '-qm', 'fixture')
  return { path, commit: git(path, 'rev-parse', 'HEAD') }
}

test('independent checkout uses exact commits and excludes dirty/ignored runtime files', () => {
  const root = mkdtempSync(join(tmpdir(), 'measix-source-replay-test-'))
  try {
    const core = repository(root, 'core'), arch = repository(root, 'arch')
    writeFileSync(join(core.path, 'source.txt'), 'newer committed source\n')
    git(core.path, 'add', 'source.txt')
    git(core.path, '-c', 'user.name=Replay test', '-c', 'user.email=replay@example.invalid', 'commit', '-qm', 'newer fixture')
    writeFileSync(join(core.path, 'source.txt'), 'uncommitted source')
    writeFileSync(join(core.path, 'private.key'), 'synthetic key not to copy')
    const result = checkoutPinnedSources({ core: core.path, architecture: arch.path, manifest: { platformCoreCommit: core.commit, architectureCommit: arch.commit }, temporaryRoot: root })
    assert.equal(readFileSync(join(result.core, 'source.txt'), 'utf8').replace(/\r\n/g, '\n'), 'pinned source\n')
    assert.equal(existsSync(join(result.core, 'private.key')), false)
    assert.equal(git(result.core, 'rev-parse', 'HEAD'), core.commit)
    assert.equal(git(result.architecture, 'rev-parse', 'HEAD'), arch.commit)
    assert.equal(git(result.core, 'status', '--porcelain'), '')
    assert.notEqual(result.core, core.path)
  } finally { rmSync(root, { recursive: true, force: true }) }
})

test('source replay rejects moving refs before creating checkouts', () => {
  assert.throws(() => checkoutPinnedSources({ core: '.', architecture: '.', manifest: { platformCoreCommit: 'HEAD', architectureCommit: 'main' } }), /full commit SHA/)
})

test('failed replay command stops later work and preserves diagnostic evidence', () => {
  const root = mkdtempSync(join(tmpdir(), 'measix-replay-command-test-'))
  try {
    const stages = [
      { id: 'failure', command: process.execPath, args: ['-e', 'console.error("expected failure");process.exit(7)'], cwd: root },
      { id: 'must-not-run', command: process.execPath, args: ['-e', 'require("node:fs").writeFileSync("unexpected", "bad")'], cwd: root },
    ]
    const results = []
    assert.throws(() => runReplayCommands(stages, root, results), /failure.*7/)
    assert.equal(results.length, 1)
    assert.equal(results[0].exitCode, 7)
    assert.match(readFileSync(join(root, 'failure.log'), 'utf8'), /expected failure/)
    assert.equal(existsSync(join(root, 'unexpected')), false)
  } finally { rmSync(root, { recursive: true, force: true }) }
})

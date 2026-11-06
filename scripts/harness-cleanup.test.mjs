import { test } from 'node:test'
import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { existsSync, mkdirSync, mkdtempSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { cleanupEnvironment } from './lib/harness.mjs'

test('cleanupEnvironment kills processes and removes the temp root synchronously', async () => {
  const envRoot = mkdtempSync(join(tmpdir(), 'measix-cleanup-'))
  mkdirSync(join(envRoot, 'nested'), { recursive: true })
  writeFileSync(join(envRoot, 'nested', 'hub.db'), 'x')

  const child = spawn(process.execPath, ['-e', 'setTimeout(() => {}, 60000)'], { stdio: 'ignore' })
  const exited = new Promise(resolve => child.once('exit', (code, signal) => resolve({ code, signal })))

  // Must complete before returning: a deferred timer is discarded whenever the
  // harness exits immediately afterwards, leaking processes and temp dirs.
  cleanupEnvironment([child], [], envRoot, false, () => {})

  assert.equal(existsSync(envRoot), false, 'temporary environment root must be removed synchronously')
  assert.equal(child.killed, true, 'child process must be signalled synchronously')

  const { code, signal } = await exited
  assert.ok(code !== 0 || signal !== null, 'child must not still be running')
})

test('cleanupEnvironment honours --keep', () => {
  const envRoot = mkdtempSync(join(tmpdir(), 'measix-cleanup-keep-'))
  writeFileSync(join(envRoot, 'marker.txt'), 'x')
  cleanupEnvironment([], [], envRoot, true, () => {})
  assert.equal(existsSync(envRoot), true, 'kept environment must be preserved')
})

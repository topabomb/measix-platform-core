import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { snapshotAdminBuild, adminBuildHash } from './lib/harness.mjs'

test('browser build snapshot retains the exact SPA while the shared build is replaced', () => {
  const fixture = mkdtempSync(join(tmpdir(), 'measix-build-snapshot-'))
  try {
    const source = join(fixture, 'console', 'dist', 'spa')
    const run = join(fixture, 'run')
    mkdirSync(join(source, 'assets'), { recursive: true })
    mkdirSync(run)
    writeFileSync(join(source, 'index.html'), '<script src="assets/app.js"></script>')
    writeFileSync(join(source, 'assets', 'app.js'), 'original app')
    const expectedHash = adminBuildHash(fixture)
    const snapshot = snapshotAdminBuild(fixture, run)
    rmSync(source, { recursive: true })
    assert.equal(readFileSync(join(snapshot.directory, 'assets', 'app.js'), 'utf8'), 'original app')
    assert.equal(readFileSync(join(snapshot.directory, 'index.html'), 'utf8'), '<script src="assets/app.js"></script>')
    assert.equal(snapshot.buildHash, expectedHash)
    assert.throws(() => snapshotAdminBuild(fixture, run), /build|index/i)
  } finally { rmSync(fixture, { recursive: true, force: true }) }
})

test('browser snapshot rejects an absent entry page or reuse of a previous run directory', () => {
  const fixture = mkdtempSync(join(tmpdir(), 'measix-build-snapshot-'))
  try {
    const source = join(fixture, 'console', 'dist', 'spa')
    const run = join(fixture, 'run')
    mkdirSync(source, { recursive: true })
    mkdirSync(run)
    assert.throws(() => snapshotAdminBuild(fixture, run), /index/i)
    writeFileSync(join(source, 'index.html'), 'app')
    snapshotAdminBuild(fixture, run)
    assert.throws(() => snapshotAdminBuild(fixture, run), /exists/i)
  } finally { rmSync(fixture, { recursive: true, force: true }) }
})

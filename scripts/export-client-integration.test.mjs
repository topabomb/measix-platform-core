import { test } from 'node:test'
import assert from 'node:assert/strict'
import { cpSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { resolve } from 'node:path'
import { execFileSync } from 'node:child_process'
import { verifyIntegration, verifyIntegrationSources } from './export-client-integration.mjs'

test('integration export verifies without sibling repositories and detects tampering or extra files', () => {
  const directory = mkdtempSync(resolve(tmpdir(), 'measix-integration-'))
  try {
    cpSync(resolve(import.meta.dirname, '../api/generated/android/integration'), directory, { recursive: true })
    const manifest = verifyIntegration(directory)
    assert.ok(Object.keys(manifest.artifacts).length > 20)
    execFileSync(process.execPath, ['verify.mjs', '--verify', '.'], { cwd: directory, windowsHide: true })
    const target = resolve(directory, 'README.md'), original = readFileSync(target)
    writeFileSync(target, 'tampered')
    assert.throws(() => verifyIntegration(directory), /digest mismatch/)
    writeFileSync(target, original)
    rmSync(target)
    assert.throws(() => verifyIntegration(directory))
    writeFileSync(target, original)
    const wrongVersion = structuredClone(manifest)
    wrongVersion.snapshotSchemaVersion = 1
    writeFileSync(resolve(directory, 'manifest.json'), JSON.stringify(wrongVersion))
    assert.throws(() => verifyIntegration(directory), /Wrong integration profile/)
    writeFileSync(resolve(directory, 'manifest.json'), JSON.stringify(manifest))
    writeFileSync(resolve(directory, 'extra'), 'unlisted')
    assert.throws(() => verifyIntegration(directory), /Unlisted/)
    rmSync(resolve(directory, 'extra'))
    const malicious = structuredClone(manifest)
    malicious.artifacts['../outside'] = 'a'.repeat(64)
    writeFileSync(resolve(directory, 'manifest.json'), JSON.stringify(malicious))
    assert.throws(() => verifyIntegration(directory), /Unsafe artifact path/)
  } finally { rmSync(directory, { recursive: true, force: true }) }
})

test('source verification rejects newly added input files missing from an old export', () => {
  const directory = mkdtempSync(resolve(tmpdir(), 'measix-source-inventory-'))
  try {
    const bundle = resolve(directory, 'bundle'), source = resolve(directory, 'source')
    cpSync(resolve(import.meta.dirname, '../api/generated/android/integration'), bundle, { recursive: true })
    cpSync(bundle, source, { recursive: true })
    verifyIntegrationSources(bundle, source)
    writeFileSync(resolve(source, 'measix-platform-core/api/fixtures/client-integration/new-case.json'), '{}')
    assert.throws(() => verifyIntegrationSources(bundle, source), /source inventory/)
  } finally { rmSync(directory, { recursive: true, force: true }) }
})

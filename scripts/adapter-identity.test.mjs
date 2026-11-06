import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { deterministicAdapterVersion } from './lib/harness.mjs'

test('adapter identity includes split protocol implementations and browser adapter source', () => {
  const root = mkdtempSync(join(tmpdir(), 'measix-adapter-identity-test-'))
  try {
    for (const dir of ['backend/test/system/adapter', 'backend/test/system/client', 'scripts/lib']) mkdirSync(join(root, dir), { recursive: true })
    writeFileSync(join(root, 'backend/test/system/adapter/adapter.go'), 'package adapter\n')
    writeFileSync(join(root, 'backend/test/system/client/client.go'), 'package client\n')
    writeFileSync(join(root, 'scripts/_server-worker.mjs'), 'adapter initial\n')
    const initial = deterministicAdapterVersion(root)
    writeFileSync(join(root, 'backend/test/system/adapter/realtime.go'), 'new protocol\n')
    const splitProtocol = deterministicAdapterVersion(root)
    assert.notEqual(splitProtocol, initial)
    writeFileSync(join(root, 'scripts/_server-worker.mjs'), 'adapter changed\n')
    assert.notEqual(deterministicAdapterVersion(root), splitProtocol)
  } finally { rmSync(root, { recursive: true, force: true }) }
})

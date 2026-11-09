import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { resolveWithin } from './lib/harness.mjs'

test('resolveWithin keeps served files inside the SPA root', () => {
  const root = mkdtempSync(join(tmpdir(), 'measix-spa-'))
  writeFileSync(join(root, 'index.html'), 'x')
  assert.equal(resolveWithin(root, 'index.html'), resolve(root, 'index.html'))
  assert.equal(resolveWithin(root, ''), resolve(root))
})

test('resolveWithin rejects traversal outside the SPA root', () => {
  const root = mkdtempSync(join(tmpdir(), 'measix-spa-'))
  for (const escape of ['..', '../..', '../outside.txt', 'a/../../outside.txt', '/etc/passwd', '../../etc/passwd']) {
    assert.equal(resolveWithin(root, escape), null, `must reject: ${escape}`)
  }
})

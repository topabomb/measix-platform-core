import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const ROOT = resolve(import.meta.dirname, '..')
const read = path => readFileSync(resolve(ROOT, path), 'utf8')

test('Caddy remains reloadable and private routes are fenced', () => {
  const config = read('deploy/preview/Caddyfile.template')
  assert.doesNotMatch(config, /\badmin\s+off\b/)
  assert.match(config, /@private\s+path \/internal \/internal\/\*/)
  assert.ok(config.indexOf('handle @private') < config.indexOf('handle {'))
})

test('preview backup uses SQLite-owned online backups and verifies recovery files', () => {
  const backup = read('deploy/preview/backup-preview.sh')
  assert.match(backup, /control-hub" backup/)
  assert.match(backup, /runtime-relay" backup/)
  assert.doesNotMatch(backup, /cp -a -- "\$root\/data\/relay\/relay-spool\.db"/)
  const verifier = read('deploy/preview/verify-backup.sh')
  assert.match(verifier, /backup checksum mismatch/)
})

test('production installer accepts only an HTTPS public origin', () => {
  const installer = read('deploy/preview/install-preview.sh')
  assert.match(installer, /\^https:\/\//)
  assert.doesNotMatch(installer, /\^https\?\:/)
})

test('Windows release packaging normalizes Linux executable modes', () => {
  const builder = read('scripts/build-preview-release.mjs')
  assert.match(builder, /createArchive\(stage, archive\)/)
  assert.match(builder, /find "\$temp" -type f -exec chmod 0644/)
  assert.match(builder, /chmod 0755 "\$temp"\/bin\/\* "\$temp"\/deploy\/\*\.sh/)
})

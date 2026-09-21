import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const ROOT = resolve(import.meta.dirname, '..')
const read = path => readFileSync(resolve(ROOT, path), 'utf8')

test('remote Caddy example fences private routes and targets the Spark tailnet address', () => {
  const config = read('deploy/preview/Caddyfile.template')
  assert.match(config, /@private\s+path \/internal \/internal\/\*/)
  assert.ok(config.indexOf('handle @private') < config.indexOf('handle {'))
  assert.match(config, /__MEASIX_SPARK_TAILSCALE_IP__:9002/)
  assert.match(config, /__MEASIX_SPARK_TAILSCALE_IP__:9004/)
  assert.doesNotMatch(config, /127\.0\.0\.1:900[24]/)
})

test('preview backup uses SQLite-owned online backups and verifies recovery files', () => {
  const backup = read('deploy/preview/backup-preview.sh')
  assert.match(backup, /control-hub" backup/)
  assert.match(backup, /runtime-relay" backup/)
  assert.match(backup, /control-hub" backup[^\n]+>&2/)
  assert.match(backup, /--output "\$target\/relay-spool\.db" >&2/)
  assert.doesNotMatch(backup, /cp -a -- "\$root\/data\/relay\/relay-spool\.db"/)
  const verifier = read('deploy/preview/verify-backup.sh')
  assert.match(verifier, /backup checksum mismatch/)
})

test('production installer accepts only an HTTPS public origin', () => {
  const installer = read('deploy/preview/install-preview.sh')
  assert.match(installer, /\^https:\/\//)
  assert.doesNotMatch(installer, /\^https\?\:/)
  assert.doesNotMatch(installer, /--env production/)
  assert.doesNotMatch(installer, /\bcaddy\b|systemctl\s+reload/)
  assert.match(installer, /pm2 start "\$root\/ecosystem\.config\.cjs"/)
})

test('PM2 uses one root-owned run script with fixed public and internal binds', () => {
  const runner = read('deploy/preview/run.sh')
  const ecosystem = read('deploy/preview/ecosystem.config.cjs')
  assert.match(runner, /--public-listen 0\.0\.0\.0:9002/)
  assert.match(runner, /--internal-listen 127\.0\.0\.1:9003/)
  assert.match(runner, /HUB_LISTEN_ADDR=0\.0\.0\.0:9004/)
  assert.match(runner, /HUB_INTERNAL_LISTEN_ADDR=127\.0\.0\.1:9001/)
  assert.match(ecosystem, /script: at\('run\.sh'\), args: \['relay'\]/)
  assert.match(ecosystem, /script: at\('run\.sh'\), args: \['hub'\]/)
  assert.doesNotMatch(ecosystem, /runtime-relay|control-hub/)
  assert.doesNotMatch(runner, /MEASIX_(?:ROOT|PUBLIC_LISTEN|INTERNAL_LISTEN)|LISTEN_(?:ADDR|PORT)=\"?\$\{/)
})

test('Windows release packaging normalizes Linux executable modes', () => {
  const builder = read('scripts/build-preview-release.mjs')
  assert.match(builder, /createArchive\(stage, archive\)/)
  assert.match(builder, /find "\$temp" -type f -exec chmod 0644/)
  assert.match(builder, /chmod 0755 "\$temp"\/bin\/\* "\$temp"\/deploy\/\*\.sh/)
})

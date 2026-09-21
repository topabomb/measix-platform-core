#!/usr/bin/env bash
set -euo pipefail

backup=${1:-}
release=${2:-}
if [[ -z "$backup" || "$backup" != /* ]]; then
  echo "usage: verify-backup.sh /absolute/backup-directory [/absolute/release-directory]" >&2
  exit 2
fi
backup=$(readlink -f -- "$backup")
[[ -d "$backup" && -f "$backup/backup-manifest.json" ]] || { echo "invalid backup directory" >&2; exit 2; }
if [[ -n "$release" ]]; then
  [[ "$release" == /* ]] || { echo "release directory must be absolute" >&2; exit 2; }
  release=$(readlink -f -- "$release")
  [[ -f "$release/release.json" && -f "$release/SHA256SUMS" ]] || { echo "invalid release directory" >&2; exit 2; }
fi

MEASIX_BACKUP_DIR="$backup" MEASIX_BACKUP_RELEASE="$release" node - <<'NODE'
const { createHash } = require('node:crypto');
const { readFileSync, statSync } = require('node:fs');
const { resolve, sep } = require('node:path');
const root = resolve(process.env.MEASIX_BACKUP_DIR);
const releaseDir = process.env.MEASIX_BACKUP_RELEASE ? resolve(process.env.MEASIX_BACKUP_RELEASE) : '';
const manifest = JSON.parse(readFileSync(resolve(root, 'backup-manifest.json'), 'utf8'));
if (manifest.formatVersion !== 1 || !manifest.release?.version || !manifest.release?.source?.coreCommit || !manifest.configVersion || !manifest.schema?.identity) throw new Error('backup manifest is incomplete');
for (const [name, expected] of Object.entries(manifest.files ?? {})) {
  const path = resolve(root, name);
  if (!path.startsWith(root + sep)) throw new Error(`backup path escapes root: ${name}`);
  const stat = statSync(path);
  if (!stat.isFile() || stat.size !== expected.size) throw new Error(`backup file size mismatch: ${name}`);
  const actual = createHash('sha256').update(readFileSync(path)).digest('hex');
  if (actual !== expected.sha256) throw new Error(`backup checksum mismatch: ${name}`);
}
if (readFileSync(resolve(root, 'config/config-version'), 'utf8').trim() !== String(manifest.configVersion)) throw new Error('backup config version mismatch');
const metadata = JSON.parse(readFileSync(resolve(root, 'hub.db.metadata.json'), 'utf8'));
if (metadata.schema !== manifest.schema.identity || metadata.schemaVersion !== manifest.schema.version) throw new Error('backup schema identity mismatch');
for (const name of ['master.key', 'jwt-ed25519.seed', 'relay-service.token']) statSync(resolve(root, 'secrets', name));
if (releaseDir) {
  const release = JSON.parse(readFileSync(resolve(releaseDir, 'release.json'), 'utf8'));
  if (release.version !== manifest.release.version || release.source?.coreCommit !== manifest.release.source.coreCommit) throw new Error('backup release identity mismatch');
  const hash = name => createHash('sha256').update(readFileSync(resolve(releaseDir, name))).digest('hex');
  if (hash('release.json') !== manifest.release.manifestSha256 || hash('SHA256SUMS') !== manifest.release.checksumsSha256) throw new Error('backup release file mismatch');
}
console.log(`backup verified: release=${manifest.release.version} schema=${manifest.schema.identity}`);
NODE

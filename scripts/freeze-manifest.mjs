#!/usr/bin/env node
/**
 * Generates the S0.1 Client Contract Freeze manifest.
 *
 * Per measix-s0-capability-delivery-contract-spec.md §10, the freeze manifest must
 * at least record:
 *   - platformCoreCommit       current Git commit of platform-core
 *   - clientControlOpenApiHash SHA-256 of api/client/client-control.openapi.yaml
 *   - canonicalFixtureHash     SHA-256 over the canonical api/fixtures tree
 *   - snapshotSchemaVersion    the frozen Android-visible schemaVersion (1)
 *
 * The manifest is written to docs/s0-freeze-manifest.json. It is a deterministic
 * build-time artifact derived from the committed source tree.
 */
import { createHash } from 'node:crypto'
import { readFileSync, readdirSync, writeFileSync, statSync } from 'node:fs'
import { join, relative, resolve } from 'node:path'
import { execFileSync } from 'node:child_process'

const ROOT = resolve(import.meta.dirname, '..')
const FIXTURES_DIR = join(ROOT, 'api', 'fixtures')
const CLIENT_OPENAPI = join(ROOT, 'api', 'client', 'client-control.openapi.yaml')
const SNAPSHOT_SCHEMA_VERSION = 1

function sha256(path) {
  // Normalize CRLF -> LF so the canonical hash matches the generator and is stable
  // across Windows/Linux working trees.
  const data = readFileSync(path).toString('utf-8').replace(/\r\n/g, '\n')
  return 'sha256:' + createHash('sha256').update(data).digest('hex')
}

function collectFiles(dir) {
  const out = []
  for (const entry of readdirSync(dir).sort()) {
    const full = join(dir, entry)
    const stat = statSync(full)
    if (stat.isDirectory()) {
      out.push(...collectFiles(full))
    } else {
      out.push(full)
    }
  }
  return out
}

function fixturesHash() {
  const files = collectFiles(FIXTURES_DIR).sort()
  const hash = createHash('sha256')
  for (const file of files) {
    hash.update(relative(FIXTURES_DIR, file).replace(/\\/g, '/'))
    hash.update('\0')
    hash.update(readFileSync(file).toString('utf-8').replace(/\r\n/g, '\n'))
    hash.update('\0')
  }
  return 'sha256:' + hash.digest('hex')
}

function gitCommit() {
  try {
    return execFileSync('git', ['rev-parse', 'HEAD'], { cwd: ROOT, encoding: 'utf-8' }).trim()
  } catch {
    return 'unknown'
  }
}

function gitDirty() {
  try {
    const status = execFileSync('git', ['status', '--porcelain'], { cwd: ROOT, encoding: 'utf-8' }).trim()
    return status.length > 0
  } catch {
    return true
  }
}

const manifest = {
  manifest: 'measix-s0-client-contract-freeze',
  snapshotSchemaVersion: SNAPSHOT_SCHEMA_VERSION,
  platformCoreCommit: gitCommit(),
  workingTreeDirty: gitDirty(),
  clientControlOpenApiHash: sha256(CLIENT_OPENAPI),
  canonicalFixtureHash: fixturesHash(),
  recordedAt: new Date().toISOString(),
}

const outPath = join(ROOT, 'docs', 's0-freeze-manifest.json')
writeFileSync(outPath, JSON.stringify(manifest, null, 2) + '\n')
process.stdout.write(`wrote ${relative(ROOT, outPath)}\n`)
process.stdout.write(JSON.stringify(manifest, null, 2) + '\n')

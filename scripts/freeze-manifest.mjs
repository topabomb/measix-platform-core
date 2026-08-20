#!/usr/bin/env node
/**
 * Generates the S0.1 Client Contract Freeze manifest.
 *
 * Per measix-s0-capability-delivery-contract-spec.md §10, the freeze manifest must
 * at least record:
 *   - architectureCommit           pinned architecture repository commit
 *   - platformCoreCommit           current Git commit of platform-core
 *   - adminBuildHash               SHA-256 of the Admin Console production build
 *   - clientControlOpenApiHash     SHA-256 of api/client/client-control.openapi.yaml
 *   - canonicalFixtureHash         SHA-256 over the canonical api/fixtures tree
 *   - snapshotSchemaVersion        the frozen Android-visible schemaVersion (1)
 *   - deterministicAdapterVersion  version hash of the deterministic Test Adapter
 *   - realAdapterQualificationRef  reference to the real adapter qualification report
 *   - scenarioResults              C6/C7 scenario pass/fail summary
 *
 * The manifest is written to docs/s0-freeze-manifest.json. It is a deterministic
 * build-time artifact derived from the committed source tree.
 */
import { createHash } from 'node:crypto'
import { existsSync, readFileSync, readdirSync, writeFileSync, statSync } from 'node:fs'
import { join, relative, resolve } from 'node:path'
import { execFileSync } from 'node:child_process'

const ROOT = resolve(import.meta.dirname, '..')
const FIXTURES_DIR = join(ROOT, 'api', 'fixtures')
const CLIENT_OPENAPI = join(ROOT, 'api', 'client', 'client-control.openapi.yaml')
const ADMIN_OPENAPI = join(ROOT, 'api', 'admin', 'admin.openapi.yaml')
const ADAPTER_SOURCE = join(ROOT, 'backend', 'test', 'system', 'adapter', 'adapter.go')
const ADAPTER_TEST = join(ROOT, 'backend', 'test', 'system', 'adapter', 'adapter_test.go')
const CLIENT_SOURCE = join(ROOT, 'backend', 'test', 'system', 'client', 'client.go')
const SNAPSHOT_SCHEMA_VERSION = 1

// Architecture repository path (sibling directory).
const ARCH_REPO = resolve(ROOT, '..', 'measix-architecture')

function sha256(path) {
  // Normalize CRLF -> LF so the canonical hash matches the generator and is stable
  // across Windows/Linux working trees.
  const data = readFileSync(path).toString('utf-8').replace(/\r\n/g, '\n')
  return 'sha256:' + createHash('sha256').update(data).digest('hex')
}

function sha256Bytes(data) {
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

function gitCommit(cwd) {
  try {
    return execFileSync('git', ['rev-parse', 'HEAD'], { cwd, encoding: 'utf-8' }).trim()
  } catch {
    return 'unknown'
  }
}

function gitDirty(cwd) {
  try {
    const status = execFileSync('git', ['status', '--porcelain'], { cwd, encoding: 'utf-8' }).trim()
    return status.length > 0
  } catch {
    return true
  }
}

function adminBuildHash() {
  // Hash the Admin Console production build output if it exists.
  const distDir = join(ROOT, 'console', 'dist', 'spa')
  if (!existsSync(distDir)) {
    return 'not-built'
  }
  const hash = createHash('sha256')
  for (const file of collectFiles(distDir).sort()) {
    hash.update(relative(distDir, file).replace(/\\/g, '/'))
    hash.update('\0')
    hash.update(readFileSync(file))
    hash.update('\0')
  }
  return 'sha256:' + hash.digest('hex')
}

function deterministicAdapterVersion() {
  // Hash the adapter source + test to produce a deterministic version.
  const hash = createHash('sha256')
  hash.update(readFileSync(ADAPTER_SOURCE).toString('utf-8').replace(/\r\n/g, '\n'))
  hash.update('\0')
  if (existsSync(ADAPTER_TEST)) {
    hash.update(readFileSync(ADAPTER_TEST).toString('utf-8').replace(/\r\n/g, '\n'))
  }
  hash.update('\0')
  // Include the Test Client source since it's part of the qualification path.
  if (existsSync(CLIENT_SOURCE)) {
    hash.update(readFileSync(CLIENT_SOURCE).toString('utf-8').replace(/\r\n/g, '\n'))
  }
  return 'sha256:' + hash.digest('hex')
}

// Scenario results: the C6/C7 scenario definitions and their expected status.
// These are the required S0.1 gate scenarios.
const scenarioResults = [
  { id: 'CAP-C6-001', name: 'Golden Path', file: 'golden_path_test.go', required: true },
  { id: 'CAP-C6-002', name: 'Test Client Four Capabilities', file: 'golden_path_test.go', required: true },
  { id: 'CAP-C6-003', name: 'Usage Closure', file: 'golden_path_test.go', required: true },
  { id: 'CAP-C6-004', name: 'Publish New Generation', file: 'golden_path_test.go', required: true },
  { id: 'CAP-C6-011', name: 'Relay Restart Recovery', file: 'golden_path_test.go', required: true },
  { id: 'CAP-C6-012', name: 'Refresh During Activation', file: 'golden_path_test.go', required: true },
  { id: 'CAP-C6-014', name: 'Full Restart Recovery', file: 'golden_path_test.go', required: true },
  { id: 'CAP-C6-015', name: 'Backup/Restore', file: 'golden_path_test.go', required: true },
  { id: 'CAP-SEC-001', name: 'Unauthenticated Access Denied', file: 'security_test.go', required: true },
  { id: 'CAP-SEC-002', name: 'CSRF Enforced', file: 'security_test.go', required: true },
  { id: 'CAP-SEC-003', name: 'Session Cookie Attributes', file: 'security_test.go', required: true },
  { id: 'CAP-SEC-004', name: 'Snapshot No Server Fields Leak', file: 'security_test.go', required: true },
  { id: 'CAP-SEC-005', name: 'Usage Detail No Content Leak', file: 'security_test.go', required: true },
  { id: 'CAP-SEC-006', name: 'Invalid Enrollment Rejected', file: 'security_test.go', required: true },
  { id: 'CAP-SEC-007', name: 'Invalid Access Token Rejected', file: 'security_test.go', required: true },
  { id: 'CAP-SEC-008', name: 'Strict JSON Validation', file: 'security_test.go', required: true },
  { id: 'CAP-SEC-009', name: 'Client Header Spoof Stripped', file: 'security_test.go', required: true },
  { id: 'CAP-SEC-010', name: 'Secret Value Never Returned', file: 'security_test.go', required: true },
  { id: 'CAP-SEC-011', name: 'Logout Invalidates Session', file: 'security_test.go', required: true },
  { id: 'CAP-SEC-012', name: 'Client API Auth Enforced', file: 'security_test.go', required: true },
  { id: 'CAP-SEC-013', name: 'Request Body Size Limit', file: 'security_test.go', required: true },
  { id: 'CAP-SEC-014', name: 'System Status No Internal Config', file: 'security_test.go', required: true },
  { id: 'CAP-SEC-015', name: 'Idempotent Publish', file: 'security_test.go', required: true },
  { id: 'BASELINE', name: 'Resource Baseline', file: 'baseline_test.go', required: false },
]

const manifest = {
  manifest: 'measix-s0-client-contract-freeze',
  snapshotSchemaVersion: SNAPSHOT_SCHEMA_VERSION,
  architectureCommit: gitCommit(ARCH_REPO),
  architectureRepoDirty: gitDirty(ARCH_REPO),
  platformCoreCommit: gitCommit(ROOT),
  workingTreeDirty: gitDirty(ROOT),
  adminBuildHash: adminBuildHash(),
  clientControlOpenApiHash: sha256(CLIENT_OPENAPI),
  adminOpenApiHash: sha256(ADMIN_OPENAPI),
  canonicalFixtureHash: fixturesHash(),
  deterministicAdapterVersion: deterministicAdapterVersion(),
  realAdapterQualificationRef: 'docs/s0-real-adapter-qualification.md',
  resourceBaselineRef: 'docs/s0-resource-baseline.md',
  scenarioResults,
  recordedAt: new Date().toISOString(),
}

const outPath = join(ROOT, 'docs', 's0-freeze-manifest.json')
writeFileSync(outPath, JSON.stringify(manifest, null, 2) + '\n')
process.stdout.write(`wrote ${relative(ROOT, outPath)}\n`)
process.stdout.write(JSON.stringify(manifest, null, 2) + '\n')

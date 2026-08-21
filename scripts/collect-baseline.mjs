#!/usr/bin/env node
/**
 * Runs the baseline test and parses the output to generate
 * .artifacts/resource-baseline.json — the machine-readable
 * artifact consumed by freeze-manifest.mjs.
 *
 * Usage: node scripts/collect-baseline.mjs
 *
 * The baseline test (backend/test/system/scenarios/baseline_test.go)
 * logs lines like:
 *   BASELINE Hub idle RSS: 12345 bytes
 *   BASELINE login latency: 12.3ms
 *   ...
 *
 * This script parses those lines and produces a JSON artifact.
 * GREEN status is computed from metric completeness — all required
 * §17 metrics must be present.
 */
import { execFileSync } from 'node:child_process'
import { writeFileSync, mkdirSync } from 'node:fs'
import { join, resolve } from 'node:path'

const ROOT = resolve(import.meta.dirname, '..')
const ARTIFACTS_DIR = join(ROOT, '.artifacts')
const OUT_PATH = join(ARTIFACTS_DIR, 'resource-baseline.json')

// Run the baseline test and capture output
console.log('Running baseline test (this may take several minutes)...')

let output
try {
  output = execFileSync('go', [
    'test',
    '-tags=candidate',
    '-run=TestBaseline',
    '-v',
    '-timeout=10m',
    './test/system/scenarios/',
  ], {
    cwd: join(ROOT, 'backend'),
    encoding: 'utf-8',
    stdio: ['pipe', 'pipe', 'pipe'],
  })
} catch (err) {
  console.error('Baseline test failed:', err.message)
  if (err.stdout) console.error(err.stdout)
  if (err.stderr) console.error(err.stderr)
  process.exit(1)
}

console.log('Parsing baseline output...')

// Parse lines like "BASELINE <name>: <value> <unit>"
const metrics = {}
const lines = output.split('\n')
for (const line of lines) {
  const match = line.match(/BASELINE\s+(.+?):\s+([\d.]+)\s*(bytes|ms|s|µs|ns)?/)
  if (match) {
    const [, name, value, unit] = match
    metrics[name.trim()] = unit ? `${value}${unit}` : `${value}`
  }
}

// Required §17 metrics — GREEN is computed from completeness.
// All required metric categories must have at least one measurement.
const requiredMetricCategories = [
  { category: 'Hub idle RSS', patterns: ['Hub idle RSS'] },
  { category: 'Relay idle RSS/CPU', patterns: ['Relay idle spool size', 'Relay idle goroutines'] },
  { category: 'Admin CRUD/Publish latency', patterns: ['login latency', 'create user latency', 'publish + activation latency'] },
  { category: 'Relay first-byte overhead', patterns: ['first-byte overhead'] },
  { category: 'Concurrent streaming memory growth', patterns: ['concurrent streaming memory growth'] },
  { category: 'Multipart memory/disk behavior', patterns: ['multipart memory/disk behavior'] },
  { category: 'Cancel release time', patterns: ['cancel release time'] },
  { category: 'Usage backlog drain', patterns: ['usage backlog drain'] },
  { category: 'SQLite growth', patterns: ['SQLite growth'] },
]

const missingCategories = []
for (const req of requiredMetricCategories) {
  const found = req.patterns.some(p => Object.keys(metrics).some(k => k.toLowerCase().includes(p.toLowerCase())))
  if (!found) {
    missingCategories.push(req.category)
  }
}

const isGreen = missingCategories.length === 0

// Get current commit
let commit = 'unknown'
try {
  commit = execFileSync('git', ['rev-parse', 'HEAD'], {
    cwd: ROOT,
    encoding: 'utf-8',
  }).trim()
} catch { /* ignore */ }

const artifact = {
  status: isGreen ? 'GREEN' : 'NOT_GREEN',
  commit,
  measuredAt: new Date().toISOString(),
  metrics,
  requiredMetrics: requiredMetricCategories.map(r => r.category),
  missingMetrics: missingCategories,
}

mkdirSync(ARTIFACTS_DIR, { recursive: true })
writeFileSync(OUT_PATH, JSON.stringify(artifact, null, 2) + '\n')
console.log(`Wrote ${OUT_PATH}`)
console.log(`Status: ${artifact.status}`)
if (missingCategories.length > 0) {
  console.log(`Missing metric categories: ${missingCategories.join(', ')}`)
}
console.log(JSON.stringify(artifact, null, 2))

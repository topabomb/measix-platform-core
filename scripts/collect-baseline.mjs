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
 *   BASELINE login latency: 12.3ms
 *   BASELINE create user latency: 45.6ms
 *   ...
 *
 * This script parses those lines and produces a JSON artifact.
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

// Parse lines like "BASELINE login latency: 12.3ms"
const metrics = {}
const lines = output.split('\n')
for (const line of lines) {
  const match = line.match(/BASELINE\s+(.+?):\s+([\d.]+)(ms|s|µs|ns)/)
  if (match) {
    const [, name, value, unit] = match
    metrics[name.trim()] = `${value}${unit}`
  }
}

// Get current commit
let commit = 'unknown'
try {
  commit = execFileSync('git', ['rev-parse', 'HEAD'], {
    cwd: ROOT,
    encoding: 'utf-8',
  }).trim()
} catch { /* ignore */ }

const artifact = {
  status: 'GREEN',
  commit,
  measuredAt: new Date().toISOString(),
  metrics,
}

mkdirSync(ARTIFACTS_DIR, { recursive: true })
writeFileSync(OUT_PATH, JSON.stringify(artifact, null, 2) + '\n')
console.log(`Wrote ${OUT_PATH}`)
console.log(JSON.stringify(artifact, null, 2))

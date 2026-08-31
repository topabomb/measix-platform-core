#!/usr/bin/env node
/**
 * Runs the baseline test and parses the output to generate
 * .artifacts/resource-baseline.json — the machine-readable
 * artifact consumed by freeze-manifest.mjs.
 *
 * Usage: node scripts/collect-baseline.mjs
 *
 * The baseline test (backend/test/system/scenarios/baseline_test.go)
 * emits a typed JSON metrics line:
 *   BASELINE_JSON_METRICS: {"hub_idle_rss_bytes":...}
 *
 * This script parses that typed JSON and produces a JSON artifact.
 * GREEN status is computed strictly from typed metric completeness —
 * all required §17 numeric metrics must be present and non-null.
 * Text log lines are kept only for human readability; they do NOT
 * contribute to the GREEN/NOT_GREEN decision.
 */
import { execFileSync } from 'node:child_process'
import { writeFileSync, mkdirSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { cpus } from 'node:os'

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

// Parse text log lines for human readability (not used for GREEN decision)
const metrics = {}
const lines = output.split('\n')
for (const line of lines) {
  const match = line.match(/BASELINE\s+(.+?):\s+([\d.]+\w*)\s*(bytes|ms|s|µs|ns|%)?/)
  if (match) {
    const [, name, value, unit] = match
    metrics[name.trim()] = unit ? `${value}${unit}` : `${value}`
  }
}

// Parse the typed JSON metrics line — this is the authoritative source
let typedMetrics = null
for (const line of lines) {
  const jsonMatch = line.match(/BASELINE_JSON_METRICS:\s*(\{.+\})/)
  if (jsonMatch) {
    try { typedMetrics = JSON.parse(jsonMatch[1]) } catch {}
    break
  }
}

// Required §17 typed metrics — GREEN is computed strictly from typed JSON
// completeness. Each required metric key must exist in typedMetrics and
// have a non-null, non-undefined value. Boolean metrics (cancel_adapter_observed)
// must be explicitly true.
//
// Sanity checks: each numeric metric is validated for reasonable ranges.
// A metric that exists but has a nonsensical value (e.g., RSS=0, CPU>100*numCores)
// is treated as missing and blocks GREEN. This prevents false GREEN from
// broken measurements.
const numCores = cpus().length || 1
const maxCpuPercent = 100 * numCores

const requiredTypedMetrics = [
  // §17.1: Hub idle RSS/CPU
  // Idle RSS should be under 500MB for a Go service; idle CPU under 50%.
  { key: 'hub_idle_rss_bytes', category: 'Hub idle RSS', mustBeNumber: true, min: 1_000_000, max: 500_000_000 },
  { key: 'hub_idle_cpu_percent', category: 'Hub idle CPU', mustBeNumber: true, min: 0, max: 50 },
  // §17.2: Relay idle RSS/CPU
  { key: 'relay_idle_rss_bytes', category: 'Relay idle RSS', mustBeNumber: true, min: 1_000_000, max: 500_000_000 },
  { key: 'relay_idle_cpu_percent', category: 'Relay idle CPU', mustBeNumber: true, min: 0, max: 50 },
  // §17.4: Relay first-byte overhead (direct vs relay)
  { key: 'direct_adapter_ttfb_ms', category: 'Direct adapter TTFB', mustBeNumber: true, min: 0, max: 60_000 },
  { key: 'relay_ttfb_ms', category: 'Relay TTFB', mustBeNumber: true, min: 0, max: 60_000 },
  { key: 'first_byte_overhead_ms', category: 'Relay first-byte overhead', mustBeNumber: true, min: -10_000, max: 60_000 },
  // §17.5: Concurrent streaming memory growth (1, 10, 50 streams)
  { key: 'concurrent_stream_mem_growth_bytes', category: 'Concurrent streaming memory growth (10)', mustBeNumber: true, min: -100_000_000, max: 1_000_000_000 },
  { key: 'concurrent_stream_50_mem_growth_bytes', category: 'Concurrent streaming memory growth (50)', mustBeNumber: true, min: -100_000_000, max: 1_000_000_000 },
  // §17.6: Multipart memory/disk behavior
  { key: 'multipart_mem_growth_bytes', category: 'Multipart memory growth', mustBeNumber: true, min: -100_000_000, max: 1_000_000_000 },
  { key: 'large_multipart_mem_growth_bytes', category: 'Large multipart memory growth', mustBeNumber: true, min: -100_000_000, max: 1_000_000_000 },
  // §17.7: Cancel release time + adapter observation
  { key: 'cancel_release_time_ms', category: 'Cancel release time', mustBeNumber: true, min: 0, max: 60_000 },
  { key: 'cancel_adapter_observed', category: 'Cancel adapter observed', mustBeBoolean: true },
  // §17.8: Hub outage → spool → drain
  { key: 'hub_outage_spool_during_bytes', category: 'Hub outage spool during', mustBeNumber: true, min: 0, max: 1_000_000_000 },
  { key: 'hub_outage_spool_drained_bytes', category: 'Hub outage spool drained', mustBeNumber: true, min: 0, max: 1_000_000_000 },
  { key: 'usage_backlog_drain_ms', category: 'Usage backlog drain', mustBeNumber: true, min: 0, max: 120_000 },
  // §17.9: SQLite growth
  { key: 'sqlite_growth_hub_bytes', category: 'SQLite growth (hub)', mustBeNumber: true, min: 0, max: 100_000_000 },
  { key: 'sqlite_growth_spool_bytes', category: 'SQLite growth (spool)', mustBeNumber: true, min: 0, max: 100_000_000 },
  // §17.x: TTS buffering
  { key: 'tts_buffering_latency_ms', category: 'TTS buffering latency', mustBeNumber: true, min: 0, max: 60_000 },
  { key: 'tts_buffering_mem_growth_bytes', category: 'TTS buffering memory growth', mustBeNumber: true, min: -100_000_000, max: 1_000_000_000 },
]

const missingCategories = []
const sanityFailures = []
if (!typedMetrics) {
  // If no typed JSON at all, everything is missing
  missingCategories.push(...requiredTypedMetrics.map(r => r.category))
} else {
  for (const req of requiredTypedMetrics) {
    const val = typedMetrics[req.key]
    if (val === undefined || val === null) {
      missingCategories.push(req.category)
      continue
    }
    if (req.mustBeNumber && typeof val !== 'number') {
      missingCategories.push(req.category)
      continue
    }
    if (req.mustBeBoolean && typeof val !== 'boolean') {
      missingCategories.push(req.category)
      continue
    }
    if (req.mustBeBoolean && val !== true) {
      missingCategories.push(req.category)
      continue
    }
    // Sanity check: numeric values must be within reasonable ranges
    if (req.mustBeNumber && typeof val === 'number') {
      if (req.min !== undefined && val < req.min) {
        sanityFailures.push(`${req.category}: value ${val} below minimum ${req.min}`)
        missingCategories.push(req.category)
        continue
      }
      if (req.max !== undefined && val > req.max) {
        sanityFailures.push(`${req.category}: value ${val} above maximum ${req.max}`)
        missingCategories.push(req.category)
        continue
      }
      // Check for NaN/Infinity
      if (!Number.isFinite(val)) {
        sanityFailures.push(`${req.category}: value ${val} is not finite`)
        missingCategories.push(req.category)
        continue
      }
    }
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
  typedMetrics,
  requiredMetrics: requiredTypedMetrics.map(r => r.category),
  missingMetrics: missingCategories,
  sanityFailures,
  numCores,
}

mkdirSync(ARTIFACTS_DIR, { recursive: true })
writeFileSync(OUT_PATH, JSON.stringify(artifact, null, 2) + '\n')
console.log(`Wrote ${OUT_PATH}`)
console.log(`Status: ${artifact.status}`)
if (missingCategories.length > 0) {
  console.log(`Missing metric categories: ${missingCategories.join(', ')}`)
}
if (sanityFailures.length > 0) {
  console.log(`Sanity failures:`)
  for (const sf of sanityFailures) {
    console.log(`  ${sf}`)
  }
}
console.log(JSON.stringify(artifact, null, 2))

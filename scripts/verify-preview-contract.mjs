#!/usr/bin/env node
import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const baseline = JSON.parse(readFileSync(join(root, 'api', 'protocol-baseline.json'), 'utf8'))
const failures = []
for (const [name, expected] of Object.entries(baseline.documents ?? {})) {
  const actual = `sha256:${createHash('sha256').update(readFileSync(join(root, name))).digest('hex')}`
  if (actual !== expected) failures.push(`${name}: expected ${expected}, actual ${actual}`)
}
if (!baseline.baseline || !baseline.policy || Object.keys(baseline.documents ?? {}).length !== 4) failures.push('baseline metadata is incomplete')
if (failures.length) {
  console.error(`Preview protocol baseline mismatch. Review compatibility and update the baseline deliberately:\n${failures.join('\n')}`)
  process.exit(1)
}
console.log(`Preview protocol baseline verified: ${baseline.baseline}`)

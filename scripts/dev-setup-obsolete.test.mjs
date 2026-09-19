import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { OBSOLETE_DATABASE_MARKERS, isObsoleteDatabaseError } from './lib/obsolete-database.mjs'

const ROOT = resolve(import.meta.dirname, '..')

test('obsolete-database markers still appear in devmigrate messages', () => {
  // Replacing an obsolete development database is driven by matching
  // devmigrate's output, so a reworded Go message must fail here rather than
  // silently stop the replacement from ever happening.
  const source = readFileSync(join(ROOT, 'backend', 'cmd', 'devmigrate', 'main.go'), 'utf8')
  const missing = OBSOLETE_DATABASE_MARKERS.filter(marker => !source.includes(marker))
  assert.deepEqual(missing, [])
  assert.ok(OBSOLETE_DATABASE_MARKERS.length >= 3, `expected several markers, found ${OBSOLETE_DATABASE_MARKERS.length}`)
})

test('only databases devmigrate reports as non-current count as replaceable', () => {
  assert.equal(isObsoleteDatabaseError('2026/09/19 13:43:14 non-current schema/checksum; delete the obsolete development database and initialize again'), true)
  assert.equal(isObsoleteDatabaseError('non-current database; delete the obsolete development database and initialize again'), true)
  assert.equal(isObsoleteDatabaseError('non-current initialization record; recreate development database: x'), true)
  // A database that is merely unrecognized, or managed by Atlas, is left alone.
  assert.equal(isObsoleteDatabaseError('unrecognized non-empty database; recreate the obsolete development database'), false)
  assert.equal(isObsoleteDatabaseError('Atlas-managed database: use Atlas, not devmigrate'), false)
  assert.equal(isObsoleteDatabaseError(''), false)
})

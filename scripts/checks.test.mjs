import { test } from 'node:test'
import assert from 'node:assert/strict'
import { commandResult, requireSuccess } from './checks.mjs'
import * as checks from './checks.mjs'
import { existsSync, mkdtempSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, dirname } from 'node:path'

test('collector fails closed for spawn errors and nonzero command exits', () => {
  for (const result of [{status: 9, stdout: 'partial'}, {status: null, error: new Error('spawn failed')}]) {
    const check = commandResult(result)
    assert.equal(check.status, 'FAIL')
    assert.throws(() => requireSuccess(check))
    assert.match(check.outputHash, /^sha256:/)
  }
  assert.equal(commandResult({status: 0, stdout: ''}).status, 'PASS')
})

test('migration replay applies and inspects the same isolated database, then cleans it', () => {
  const parent = mkdtempSync(join(tmpdir(), 'measix replay test '))
  const calls = []
  try {
    checks.replayMigrations({ temporaryRoot: parent, run: (command, args) => {
      assert.equal(command, 'atlas')
      const url = args[args.indexOf('--url') + 1]
      const path = url.slice('sqlite://'.length)
      if (args[1] === 'apply') writeFileSync(path, 'test database')
      if (args[1] === 'status') assert.equal(existsSync(path), true)
      calls.push({ operation: args[1], url, path })
    } })
    assert.deepEqual(calls.map(c => c.operation), ['apply', 'status'])
    assert.equal(calls[0].url, calls[1].url)
    assert.equal(existsSync(dirname(calls[0].path)), false)
  } finally { rmSync(parent, { recursive: true, force: true }) }
})

test('migration replay stops at an apply failure and still cleans its temp directory', () => {
  let directory
  let calls = 0
  assert.throws(() => checks.replayMigrations({ run: (_command, args) => {
    calls++
    directory = dirname(args[args.indexOf('--url') + 1].slice('sqlite://'.length))
    throw new Error('apply rejected')
  } }), /apply rejected/)
  assert.equal(calls, 1)
  assert.equal(existsSync(directory), false)
})

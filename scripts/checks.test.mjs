import { test } from 'node:test'
import assert from 'node:assert/strict'
import { commandResult, requireSuccess } from './checks.mjs'

test('collector fails closed for spawn errors and nonzero command exits', () => {
  for (const result of [{status: 9, stdout: 'partial'}, {status: null, error: new Error('spawn failed')}]) {
    const check = commandResult(result)
    assert.equal(check.status, 'FAIL')
    assert.throws(() => requireSuccess(check))
    assert.match(check.outputHash, /^sha256:/)
  }
  assert.equal(commandResult({status: 0, stdout: ''}).status, 'PASS')
})

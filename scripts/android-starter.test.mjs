import { test } from 'node:test'
import assert from 'node:assert/strict'
import { Worker } from 'node:worker_threads'
import { once } from 'node:events'
import { freePort } from './lib/harness.mjs'
import { assertInstrumentationPassed, verifyStarterRequest, validateAndroidSerial } from './lib/android-starter.mjs'

const assistantSystem = 'Original assistant instructions'
const starter = { starterId: 'str_synthetic', prompt: 'Question', openingSnapshot: { format: 1, systemPrompt: 'Domain system', initialContexts: [{ id: 'b2', title: 'Second', content: 'second content' }, { id: 'b1', title: 'First', content: 'first content' }] } }
const request = { model: 'model', messages: [{ role: 'system', content: 'Rules\nDomain system' }, { role: 'user', content: JSON.stringify({ type: 'starter_context', format: 1, blocks: starter.openingSnapshot.initialContexts }) }, { role: 'user', content: 'Question' }] }

test('instrumentation requires one completed test and rejects failure, ignored, zero-test and adb-only success', () => {
  assertInstrumentationPassed('INSTRUMENTATION_STATUS_CODE: 1\nINSTRUMENTATION_STATUS_CODE: 0\nOK (1 test)\nINSTRUMENTATION_CODE: -1')
  for (const output of ['', 'OK (0 tests)', 'INSTRUMENTATION_STATUS_CODE: -3\nOK (1 test)', 'INSTRUMENTATION_STATUS_CODE: 0\nFAILURES!!!\nOK (1 test)', 'INSTRUMENTATION_STATUS_CODE: 0\nOK (2 tests)']) {
    assert.throws(() => assertInstrumentationPassed(output))
  }
})
test('wire verification checks system, source order, exact prompt, and preserves literal text', () => {
  assert.deepEqual(verifyStarterRequest([request], starter, assistantSystem).contextIds, ['b2', 'b1'])
  const canonical = { ...starter, openingSnapshot: { ...starter.openingSnapshot, initialContexts: starter.openingSnapshot.initialContexts.map(({ id, title, content }) => ({ content, id, title })) } }
  assert.deepEqual(verifyStarterRequest([request], canonical, assistantSystem).contextIds, ['b2', 'b1'])
  const merged = { model: 'model', messages: [request.messages[0], { role: 'user', content: request.messages[1].content + '\n\nQuestion' }] }
  assert.deepEqual(verifyStarterRequest([merged], starter, assistantSystem).contextIds, ['b2', 'b1'])
  for (const invalid of [
    { ...request, messages: request.messages.slice(1) },
    { ...request, messages: [{ role: 'system', content: 'Domain system' }, { role: 'user', content: 'First first content Second second content' }, { role: 'user', content: 'Question' }] },
    { ...request, messages: [...request.messages.slice(0, 2), { role: 'user', content: 'Changed prompt' }] },
  ]) assert.throws(() => verifyStarterRequest([invalid], starter, assistantSystem))
})
test('Android lane requires the dedicated fixture emulator and cannot target the production demo', () => {
  assert.equal(validateAndroidSerial('emulator-5562'), 'emulator-5562')
  for (const serial of ['', 'emulator-5560', 'device', 'emulator-5562; rm']) assert.throws(() => validateAndroidSerial(serial))
})

test('opt-in worker captures only selected synthetic model bodies through actual HTTP', async () => {
  const adapterPort = await freePort(), spaPort = await freePort()
  const worker = new Worker(new URL('./_server-worker.mjs', import.meta.url), { workerData: { adapterPort, spaPort, hubPort: 1, relayPort: 1, spaDir: '.', captureStarterRequests: true } })
  try {
    await once(worker, 'message')
    const started = once(worker, 'message')
    worker.postMessage({ starterCapture: 'start', id: 'start', prompt: starter.prompt })
    assert.equal((await started)[0].started, true)
    for (const body of [{ messages: [{ role: 'user', content: 'Other request' }] }, request]) {
      const response = await fetch(`http://127.0.0.1:${adapterPort}/v1/chat/completions`, {
        method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: 'Bearer synthetic-secret-not-captured' }, body: JSON.stringify(body),
      })
      assert.equal(response.status, 200)
      await response.text()
    }
    const stopped = once(worker, 'message')
    worker.postMessage({ starterCapture: 'stop', id: 'stop' })
    const captured = (await stopped)[0]
    assert.equal(captured.requests.length, 1)
    assert.equal(captured.overflow, false)
    assert.equal(JSON.stringify(captured).includes('synthetic-secret-not-captured'), false)
    assert.deepEqual(verifyStarterRequest(captured.requests, starter, assistantSystem).contextIds, ['b2', 'b1'])
  } finally {
    await worker.terminate()
  }
})

test('wire verification rejects concatenating original Assistant instructions with the Starter System', () => {
  const original = 'Original assistant instructions'
  const contaminated = { ...request, messages: [{ role: 'system', content: `${original}\nRules\nDomain system` }, ...request.messages.slice(1)] }
  assert.throws(() => verifyStarterRequest([contaminated], starter, original), /Assistant/)
})

test('System replacement allows intentional copying, including empty or unchanged authored openings', () => {
  const unchanged = { ...starter, openingSnapshot: { ...starter.openingSnapshot, systemPrompt: assistantSystem } }
  const copied = { ...request, messages: [{ role: 'system', content: `Rules\n${assistantSystem}` }, ...request.messages.slice(1)] }
  assert.doesNotThrow(() => verifyStarterRequest([copied], unchanged, assistantSystem))
  assert.throws(() => verifyStarterRequest([{ ...copied, messages: [{ role: 'system', content: `${assistantSystem}\n${assistantSystem}` }, ...request.messages.slice(1)] }], unchanged, assistantSystem), /Assistant/)
  const empty = { ...starter, openingSnapshot: { ...starter.openingSnapshot, systemPrompt: '' } }
  assert.doesNotThrow(() => verifyStarterRequest([{ ...request, messages: [{ role: 'system', content: 'Rules' }, ...request.messages.slice(1)] }], empty, assistantSystem))
  assert.throws(() => verifyStarterRequest([copied], empty, assistantSystem), /Assistant/)
  assert.throws(() => verifyStarterRequest([request], starter), /Assistant/)
})

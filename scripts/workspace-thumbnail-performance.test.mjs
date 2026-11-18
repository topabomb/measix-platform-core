import test from 'node:test'
import assert from 'node:assert/strict'
import { performanceFixturePlan, positiveTransferOverlap, runSampledPhase, seedThumbnailPerformance, summarizePerformance, validatePerformanceFixture } from './workspace-thumbnail-performance.mjs'

function clockFixture() {
  let current = 0
  const timers = new Set()
  return {
    now: () => current,
    wait: (milliseconds, signal) => new Promise((resolve, reject) => {
      const timer = { at: current + milliseconds, resolve, abort: () => { timers.delete(timer); reject(signal.reason) } }
      timers.add(timer)
      signal.addEventListener('abort', timer.abort, { once: true })
      timer.resolve = () => { timers.delete(timer); signal.removeEventListener('abort', timer.abort); resolve() }
      if (signal.aborted) timer.abort()
    }),
    async tick(milliseconds) {
      current += milliseconds
      for (const timer of [...timers]) if (timer.at <= current) timer.resolve()
      await new Promise(setImmediate)
    },
    pending: () => timers.size,
  }
}

test('native overlap excludes equal endpoints and zero-width intervals, unions positive time, and rejects reversed clocks', () => {
  assert.deepEqual(positiveTransferOverlap({ start: 10, end: 20 }, [{ start: 0, end: 10 }, { start: 20, end: 30 }, { start: 15, end: 15 }]), { overlappingThumbnailRequests: 0, overlapMs: 0 })
  assert.deepEqual(positiveTransferOverlap({ start: 10, end: 10 }, [{ start: 0, end: 30 }]), { overlappingThumbnailRequests: 0, overlapMs: 0 })
  assert.deepEqual(positiveTransferOverlap({ start: 10, end: 20 }, [{ start: 0, end: 15 }, { start: 12, end: 22 }]), { overlappingThumbnailRequests: 2, overlapMs: 10 })
  assert.throws(() => positiveTransferOverlap({ start: 20, end: 10 }, []), /timing/)
  assert.throws(() => positiveTransferOverlap({ start: 10, end: 20 }, [{ start: 18, end: 17 }]), /timing/)
})

test('phase samples a correctly sized current-directory image during a held action and stops after completion', async () => {
  const clock = clockFixture(), path = 'thumbnail-performance/a.png'
  const phase = { name: 'cards', startedAt: 0, endedAt: 0, visiblePaths: [], renderedPaths: [] }
  let release, manualSample, observations = 0
  const held = new Promise(resolve => { release = resolve })
  const running = runSampledPhase({ phase, directory: 'thumbnail-performance', expectedSize: 256, now: clock.now, wait: clock.wait,
    read: async () => { observations++; return { directory: clock.now() === 50 ? 'old-directory' : 'thumbnail-performance', size: clock.now() < 50 ? 128 : 256, visible: [path, 'other/b.png'], rendered: [path, 'other/b.png'] } },
    action: async sample => { manualSample = sample; await held }, settle: async sample => { await sample() },
  })
  try {
    await clock.tick(0)
    assert.equal(phase.firstImageAt, undefined, 'old 128px image was counted for a 256px phase')
    await clock.tick(50); await manualSample()
    assert.equal(phase.firstImageAt, undefined, 'old directory was counted for the current phase')
    await clock.tick(50)
    assert.equal(phase.firstImageAt, 100, 'first image was delayed until the unrelated action completed')
    assert.equal(phase.allVisibleAt, 100)
    assert.deepEqual(phase.visiblePaths, [path])
    await clock.tick(400)
    assert.equal(phase.firstImageAt, 100)
  } finally { release(); await running }
  assert.equal(phase.allVisibleAt, 100, 'repeated complete samples moved the completion milestone')
  assert.equal(clock.pending(), 0)
  const stoppedAt = observations
  await clock.tick(1000)
  assert.equal(observations, stoppedAt)
})

test('phase action failure joins and stops its sampler without losing the original error', async () => {
  const clock = clockFixture(), path = 'thumbnail-performance/a.png'
  const phase = { name: 'failure', startedAt: 0, endedAt: 0, visiblePaths: [], renderedPaths: [] }
  let rejectAction, observations = 0
  const held = new Promise((_, reject) => { rejectAction = reject })
  const running = runSampledPhase({ phase, directory: 'thumbnail-performance', expectedSize: 128, now: clock.now, wait: clock.wait,
    read: async () => { observations++; return { directory: 'thumbnail-performance', size: 128, visible: [path], rendered: [path] } },
    action: async () => { await held }, settle: async () => { throw Error('settle must not run') },
  })
  const rejected = assert.rejects(running, /original action failure/)
  await clock.tick(0); await clock.tick(100)
  const observedDuringAction = observations
  rejectAction(Error('original action failure')); await rejected
  assert.ok(observedDuringAction >= 2, 'sampler did not run while the action was pending')
  assert.equal(clock.pending(), 0)
  await clock.tick(1000)
  assert.equal(observations, observedDuringAction)
})

test('a periodic DOM-read failure is propagated and leaves no sampler waiting behind the action', async () => {
  const clock = clockFixture(), path = 'thumbnail-performance/a.png'
  const phase = { name: 'read-failure', startedAt: 0, endedAt: 0, visiblePaths: [], renderedPaths: [] }
  let release, observations = 0
  const held = new Promise(resolve => { release = resolve })
  const running = runSampledPhase({ phase, directory: 'thumbnail-performance', expectedSize: 128, now: clock.now, wait: clock.wait,
    read: async () => { if (++observations > 1) throw Error('DOM read failed'); return { directory: 'thumbnail-performance', size: 128, visible: [path], rendered: [path] } },
    action: async () => { await held }, settle: async sample => { await sample() },
  })
  const rejected = assert.rejects(running, /DOM read failed/)
  await clock.tick(0); await clock.tick(100)
  release(); await rejected
  assert.equal(clock.pending(), 0)
  const stoppedAt = observations
  await clock.tick(1000)
  assert.equal(observations, stoppedAt)
})

test('performance seed contains forty distinct mixed images including 2–8 MP sources', () => {
  const plan = performanceFixturePlan()
  assert.equal(plan.length, 40)
  assert.equal(new Set(plan.map(item => item.path)).size, 40)
  assert.equal(plan.filter(item => item.path.endsWith('.png')).length, 20)
  assert.equal(plan.filter(item => item.path.endsWith('.jpg')).length, 20)
  assert.ok(plan.some(item => item.width * item.height >= 2_000_000))
  assert.ok(plan.some(item => item.width * item.height >= 7_000_000))
  assert.ok(plan.every(item => item.width * item.height <= 8_000_000))
})

test('metrics keep cancellation, 429 and ordinary errors separate and never imply coverage from requests', () => {
  const value = summarizePerformance({
    phases: [{ name: 'cold', startedAt: 100, endedAt: 180, firstImageAt: 130, allVisibleAt: 170, visiblePaths: ['a', 'b'], renderedPaths: ['a'] }],
    requests: [
      { method: 'GET', phase: 'cold', startedAt: 101, endedAt: 140, status: 200 },
      { method: 'GET', phase: 'cold', startedAt: 102, endedAt: 110, status: 429 },
      { method: 'GET', phase: 'cold', startedAt: 111, endedAt: 120, status: 200, cancelled: true },
      { method: 'GET', phase: 'cold', startedAt: 121, endedAt: 150, status: 503 },
      { method: 'GET', phase: 'cold', startedAt: 151 },
      { method: 'OPTIONS', phase: 'cold', startedAt: 100, endedAt: 101, status: 200 },
    ],
  })
  assert.deepEqual(value.requests, { total: 5, success: 1, busy: 1, cancelled: 1, failed: 1, pending: 1 })
  assert.equal(value.capabilityQueries, 1)
  assert.equal(value.phases[0].visible, 2)
  assert.equal(value.phases[0].rendered, 1)
  assert.equal(value.phases[0].coverage, 0.5)
  assert.equal(value.phases[0].firstImageMs, 30)
  assert.equal(value.phases[0].allVisibleMs, null)
  assert.equal(value.phases[0].requests.busy, 1)
})

test('seed configuration rejects a non-D output without generating or uploading anything', async () => {
  let uploads = 0
  await assert.rejects(seedThumbnailPerformance({ out: 'C:\\invalid-performance-output', mediaDirectory: 'C:\\invalid-performance-output\\media', users: [], put: async () => { uploads++ } }), /D:/)
  assert.equal(uploads, 0)
})

test('empty render evidence is not 100% coverage and invalid clocks are rejected', () => {
  const input = { phases: [{ name: 'empty', startedAt: 0, endedAt: 5, visiblePaths: [], renderedPaths: [] }], requests: [] }
  assert.equal(summarizePerformance(input).phases[0].coverage, null)
  assert.throws(() => summarizePerformance({ ...input, phases: [{ ...input.phases[0], endedAt: -1 }] }), /timing/)
  assert.throws(() => summarizePerformance({ ...input, requests: [{ method: 'GET', startedAt: 9, endedAt: 8 }] }), /timing/)
})

test('fixture preflight rejects missing or malformed media before any upload', () => {
  assert.throws(() => validatePerformanceFixture({ entries: [] }), /fixture/)
  const plan = performanceFixturePlan().map(item => ({ ...item, kind: 'IMAGE', bytes: 20, sha256: '0'.repeat(64) }))
  const videos = ['40-video.mp4', '41-video.webm'].map(file => ({ path: 'thumbnail-performance/' + file, kind: 'VIDEO', bytes: 20, sha256: '0'.repeat(64) }))
  const fixture = { schemaVersion: 1, directory: 'thumbnail-performance', entries: [...plan, ...videos], range: { path: 'thumbnail-performance/range.bin', bytes: 4 * 1024 * 1024, start: 1024 * 1024, length: 1024 * 1024, sha256: '0'.repeat(64) } }
  assert.equal(validatePerformanceFixture(fixture), fixture)
  assert.throws(() => validatePerformanceFixture({ ...fixture, entries: [...fixture.entries.slice(1), fixture.entries[1]] }), /fixture/)
  assert.throws(() => validatePerformanceFixture({ ...fixture, range: { ...fixture.range, path: '../range.bin' } }), /fixture/)
})

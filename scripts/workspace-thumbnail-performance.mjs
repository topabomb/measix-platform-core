/** Workload/metrics for the existing isolated thumbnail runner. This module
 * does not create accounts, processes that outlive generation, or resources. */
import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { execFileSync } from 'node:child_process'
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join, resolve, sep } from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'

const directory = 'thumbnail-performance'
const digest = bytes => createHash('sha256').update(bytes).digest('hex')

export function performanceFixturePlan() {
  const dimensions = [[640, 480], [1920, 1200], [2560, 1440], [3200, 2400]]
  return Array.from({ length: 40 }, (_, index) => {
    // Each dimension occurs in both encodings; no filename aliases a source.
    const [width, height] = dimensions[Math.floor(index / 2) % dimensions.length]
    return { path: `${directory}/${String(index).padStart(2, '0')}-image.${index % 2 ? 'jpg' : 'png'}`, width, height }
  })
}

export function validatePerformanceFixture(value) {
  assert.ok(value?.schemaVersion === 1 && value.directory === directory && Array.isArray(value.entries) && value.entries.length === 42, 'Invalid performance fixture')
  const plan = performanceFixturePlan()
  const paths = new Set()
  for (const [index, entry] of value.entries.entries()) {
    const expected = index < 40 ? plan[index] : { path: `${directory}/${index === 40 ? '40-video.mp4' : '41-video.webm'}` }
    assert.ok(entry.path === expected.path && !paths.has(entry.path) && entry.kind === (index < 40 ? 'IMAGE' : 'VIDEO') && Number.isSafeInteger(entry.bytes) && entry.bytes > 0 && /^[0-9a-f]{64}$/.test(entry.sha256 ?? ''), 'Invalid performance fixture media')
    if (index < 40) assert.ok(entry.width === expected.width && entry.height === expected.height, 'Invalid performance fixture dimensions')
    paths.add(entry.path)
  }
  const range = value.range
  assert.ok(range?.path === `${directory}/range.bin` && range.bytes === 4 * 1024 * 1024 && range.start === 1024 * 1024 && range.length === 1024 * 1024 && /^[0-9a-f]{64}$/.test(range.sha256 ?? ''), 'Invalid performance fixture Range')
  return value
}

// Pillow is an existing qualification dependency, as in the fixture bridge.
// These images are deterministic gradients/geometry with real 0.3–7.7 MP
// decoding work; they are not a photographic corpus or an entropy benchmark.
const generateImages = String.raw`
import json, pathlib, sys
from PIL import Image, ImageDraw
target=pathlib.Path(sys.argv[1])
for index, item in enumerate(json.loads(sys.stdin.read())):
    width,height=item['width'],item['height']
    image=Image.new('RGB',(width,height))
    drawing=ImageDraw.Draw(image)
    for y in range(height):
        drawing.line((0,y,width,y), fill=((y*255//height+index*13)%256,(y*127//height+index*7)%256,(y*63//height+index*19)%256))
    for band in range(16):
        x=(band*width//16+index*11)%width
        drawing.rectangle((x,height//4,min(width-1,x+max(1,width//48)),height*3//4),fill=((index*17+band*11)%256,(band*29)%256,220))
    path=target/pathlib.Path(item['path']).name
    image.save(path, quality=88, optimize=False)
    image.close()
`

export async function seedThumbnailPerformance({ out, mediaDirectory, users, put }) {
  const output = resolve(out), media = resolve(mediaDirectory), target = join(output, 'performance-media')
  assert.match(output, /^D:\\/i, 'Performance evidence must remain on D:')
  assert.ok(media.startsWith(output + sep) && existsSync(media), 'Use this run-owned fixture media directory')
  assert.ok(!existsSync(target) && Array.isArray(users) && users.length === 2 && users.every(user => user.userId && user.spaceId) && typeof put === 'function', 'Invalid performance seed inputs')
  for (const name of ['video.mp4', 'video.webm']) assert.ok(readFileSync(join(media, name)).length > 0, 'Missing real video fixture')
  mkdirSync(target)
  const plan = performanceFixturePlan()
  const wslTarget = '/mnt/d/' + target.slice(3).replaceAll('\\', '/')
  execFileSync('wsl.exe', ['-d', 'Ubuntu', '--', 'python3', '-B', '-c', generateImages, wslTarget], { input: JSON.stringify(plan), encoding: 'utf8', windowsHide: true, timeout: 60000, maxBuffer: 1024 * 1024 })
  const entries = plan.map(item => {
    const bytes = readFileSync(join(target, item.path.split('/').pop()))
    return { ...item, kind: 'IMAGE', bytes: bytes.length, sha256: digest(bytes) }
  })
  for (const [index, name] of ['video.mp4', 'video.webm'].entries()) {
    const bytes = readFileSync(join(media, name)), path = `${directory}/${40 + index}-${name}`
    writeFileSync(join(target, path.split('/').pop()), bytes, { flag: 'wx' })
    entries.push({ path, kind: 'VIDEO', bytes: bytes.length, sha256: digest(bytes) })
  }
  const rangeBytes = Buffer.alloc(4 * 1024 * 1024)
  for (let index = 0; index < rangeBytes.length; index++) rangeBytes[index] = (index * 17 + Math.floor(index / 65536)) % 251
  const range = { path: `${directory}/range.bin`, bytes: rangeBytes.length, start: 1024 * 1024, length: 1024 * 1024, sha256: digest(rangeBytes.subarray(1024 * 1024, 2 * 1024 * 1024)) }
  writeFileSync(join(target, 'range.bin'), rangeBytes, { flag: 'wx' })
  const fixture = validatePerformanceFixture({ schemaVersion: 1, directory, entries, range })
  // Validate every source and the complete inventory before the first upload.
  for (const user of users) {
    for (const entry of entries) await put(user, entry.path, readFileSync(join(target, entry.path.split('/').pop())))
    await put(user, range.path, rangeBytes)
  }
  writeFileSync(join(output, 'performance-fixture.json'), JSON.stringify(fixture, null, 2) + '\n', { flag: 'wx' })
  return fixture
}

function interval(start, end) {
  assert.ok(Number.isFinite(start) && Number.isFinite(end) && end >= start, 'Invalid performance timing')
}
export function positiveTransferOverlap(range, transfers) {
  interval(range.start, range.end)
  const overlapping = []
  for (const item of transfers) {
    interval(item.start, item.end)
    const start = Math.max(range.start, item.start), end = Math.min(range.end, item.end)
    if (end > start) overlapping.push({ start, end })
  }
  overlapping.sort((left, right) => left.start - right.start)
  let overlapMs = 0, until = range.start
  for (const item of overlapping) {
    overlapMs += Math.max(0, item.end - Math.max(until, item.start))
    until = Math.max(until, item.end)
  }
  return { overlappingThumbnailRequests: overlapping.length, overlapMs }
}
export async function runSampledPhase({ phase, directory, expectedSize, read, action, settle, now = Date.now, wait = (ms, signal) => delay(ms, undefined, { signal }) }) {
  assert.ok(directory && [128, 256].includes(expectedSize), 'Invalid phase sampling scope')
  const controller = new AbortController()
  let pending, samplingError, actionFailed = false
  phase.sampleIntervalMs = 100; phase.sampledAt = []
  function sample() {
    return pending ??= (async () => {
      const value = await read(), at = now()
      if (controller.signal.aborted) return { visible: [], rendered: [] }
      phase.sampledAt.push(at)
      // Mode and directory transitions may expose the previous view for a
      // frame. Such images cannot establish this phase's first/all milestone.
      const visible = value.directory === directory && value.size === expectedSize
        ? value.visible.filter(path => path.startsWith(directory + '/') && !path.slice(directory.length + 1).includes('/')) : []
      const rendered = value.rendered.filter(path => visible.includes(path))
      const introduced = visible.some(path => !phase.visiblePaths.includes(path))
      phase.visiblePaths = [...new Set([...phase.visiblePaths, ...visible])]
      phase.renderedPaths = [...new Set([...phase.renderedPaths, ...rendered])]
      if (rendered.length && phase.firstImageAt === undefined) phase.firstImageAt = at
      const complete = phase.visiblePaths.length > 0 && phase.visiblePaths.every(path => phase.renderedPaths.includes(path))
      if (!complete) delete phase.allVisibleAt
      else if (introduced || phase.allVisibleAt === undefined) phase.allVisibleAt = at
      return { visible, rendered }
    })().finally(() => { pending = undefined })
  }
  // Join every sample; periodic and explicit scroll checks share one in-flight
  // DOM read. Abortable waiting leaves no timer or rejected background task.
  const sampling = (async () => {
    while (!controller.signal.aborted) {
      try { await sample(); await wait(100, controller.signal) }
      catch (error) { if (!controller.signal.aborted) samplingError = error; break }
    }
  })()
  try {
    await action(sample)
    if (samplingError) throw samplingError
    await settle(sample)
  } catch (error) { actionFailed = true; throw error }
  finally {
    controller.abort(); await sampling
    await pending?.catch(() => {})
    phase.endedAt = now()
    if (!actionFailed && samplingError) throw samplingError
  }
}
function counts(requests) {
  const value = { total: 0, success: 0, busy: 0, cancelled: 0, failed: 0, pending: 0 }
  for (const item of requests.filter(item => item.method === 'GET')) {
    value.total++
    if (item.cancelled) value.cancelled++
    else if (item.endedAt === undefined) value.pending++
    else if (item.status === 429) value.busy++
    else if (item.status === 200 && !item.failed) value.success++
    else value.failed++
  }
  return value
}
function durations(requests) {
  const values = requests.filter(item => item.method === 'GET' && item.status === 200 && !item.cancelled && !item.failed && item.endedAt !== undefined).map(item => item.endedAt - item.startedAt).sort((a, b) => a - b)
  const percentile = rank => values.length ? values[Math.max(0, Math.ceil(values.length * rank) - 1)] : null
  return { count: values.length, p50Ms: percentile(0.5), p95Ms: percentile(0.95) }
}
export function summarizePerformance({ phases, requests }) {
  assert.ok(Array.isArray(phases) && Array.isArray(requests), 'Missing performance observations')
  for (const item of requests) {
    assert.ok(Number.isFinite(item.startedAt), 'Invalid performance timing')
    if (item.endedAt !== undefined) interval(item.startedAt, item.endedAt)
  }
  return {
    requests: counts(requests), capabilityQueries: requests.filter(item => item.method === 'OPTIONS').length,
    observedBrowserEventDuration: durations(requests),
    phases: phases.map(phase => {
      interval(phase.startedAt, phase.endedAt)
      const offset = value => { if (value === undefined) return null; interval(phase.startedAt, value); interval(value, phase.endedAt); return value - phase.startedAt }
      const visible = new Set(phase.visiblePaths), rendered = new Set(phase.renderedPaths.filter(path => visible.has(path)))
      const phaseRequests = requests.filter(item => item.phase === phase.name)
      const coverage = visible.size ? rendered.size / visible.size : null
      return { name: phase.name, elapsedMs: phase.endedAt - phase.startedAt, firstImageMs: offset(phase.firstImageAt), allVisibleMs: coverage === 1 ? offset(phase.allVisibleAt) : null, visible: visible.size, rendered: rendered.size, coverage, requests: counts(phaseRequests), observedBrowserEventDuration: durations(phaseRequests) }
    }),
  }
}

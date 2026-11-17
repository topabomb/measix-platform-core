import { onBeforeUnmount, onMounted, shallowReactive, watch, type ComponentPublicInstance } from 'vue'
import { ApiProblem, apiResponse } from '../api/client'
import type { components } from '../api/generated'

type Entry = components['schemas']['WorkspaceFileEntry']
type Capabilities = components['schemas']['WorkspaceThumbnailCapabilities']
type Demand = { entry: Entry; readonly sourceETag: string; key: string; visible: boolean; failed: boolean; retries: number; job?: Job }
type Job = { controller: AbortController }
type Options = {
  identity: () => string
  baseUrl: () => string
  spaceId: () => string
  revision?: () => string
  available: () => boolean
  entries: () => Entry[]
  size?: () => 128 | 256
}
const bodyLimit = 1024 * 1024

export function thumbnailMediaKind(path: string): 'IMAGE' | 'VIDEO' | undefined {
  const ext = path.split('.').pop()?.toLowerCase()
  if (['jpg', 'jpeg', 'png', 'apng', 'gif', 'webp', 'bmp'].includes(ext ?? '')) return 'IMAGE'
  if (['mp4', 'm4v', 'mov', 'webm', 'ogv'].includes(ext ?? '')) return 'VIDEO'
}
function mediaKind(entry: Entry) {
  if (entry.kind !== 'FILE' || !entry.etag || !/^"[\x21\x23-\x7e\x80-\xff]*"$/.test(entry.etag)) return
  return thumbnailMediaKind(entry.path)
}

async function boundedBytes(response: Response, limit: number, signal: AbortSignal) {
  if (Number(response.headers.get('Content-Length')) > limit) {
    await response.body?.cancel()
    throw new Error('Thumbnail response too large')
  }
  const reader = response.body?.getReader()
  if (!reader) throw new Error('Missing thumbnail response')
  const abort = () => { void reader.cancel().catch(() => {}) }
  signal.addEventListener('abort', abort, { once: true })
  const chunks: Uint8Array[] = []
  let length = 0
  try {
    if (signal.aborted) throw new DOMException('Thumbnail cancelled', 'AbortError')
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      length += value.length
      if (length > limit) throw new Error('Thumbnail response too large')
      chunks.push(value)
    }
  } finally { signal.removeEventListener('abort', abort); await reader.cancel() }
  const bytes = new Uint8Array(length)
  let offset = 0
  for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.length }
  return bytes
}

export function useWorkspaceThumbnails(options: Options) {
  const urls = shallowReactive(new Map<string, string>())
  // A changed demand must rebind its row even before any URL is available.
  const demands = shallowReactive(new Map<string, Demand>())
  const targets = new Map<string, Element>()
  const jobs = new Set<Job>()
  let observer: IntersectionObserver | undefined
  let capability: Capabilities | undefined
  let capabilityController: AbortController | undefined
  let capabilityAttempted = false
  let discoveryQueued = false
  let mounted = false, alive = true, scopeEpoch = 0
  let concurrency = 2, cooldownUntil = 0
  let cooldownTimer: ReturnType<typeof setTimeout> | undefined
  const size = () => options.size?.() ?? 128
  const scope = () => JSON.stringify([options.identity(), options.baseUrl(), options.spaceId(), options.revision?.() ?? ''])
  const key = (entry: Entry) => JSON.stringify([scope(), entry.path, entry.etag, size()])
  const enabled = () => alive && mounted && !!options.identity() && !!options.spaceId() && options.available() && !document.hidden

  function stopCooldown() {
    if (cooldownTimer !== undefined) clearTimeout(cooldownTimer)
    cooldownTimer = undefined
  }
  function resumeCooldown() {
    stopCooldown()
    if (!enabled()) return
    const remaining = cooldownUntil - Date.now()
    if (remaining > 0) cooldownTimer = setTimeout(resumeCooldown, Math.min(remaining, 2147483647))
    else { cooldownUntil = 0; pump() }
  }

  function release(demand: Demand) {
    demand.job?.controller.abort()
    const url = urls.get(demand.key)
    if (url) { URL.revokeObjectURL(url); urls.delete(demand.key) }
  }
  function clearDirectory() {
    for (const demand of demands.values()) release(demand)
    demands.clear()
    observer?.disconnect()
    targets.clear()
  }
  function syncEntries() {
    const entries = options.entries().filter(entry => mediaKind(entry))
    const wanted = new Map(entries.map(entry => [entry.path, entry]))
    for (const [path, demand] of demands) {
      if (!wanted.has(path) || key(wanted.get(path)!) !== demand.key) {
        release(demand); demands.delete(path)
        const target = targets.get(path)
        if (target) observer?.unobserve(target)
        targets.delete(path)
      }
    }
    for (const entry of entries) {
      if (!demands.has(entry.path)) demands.set(entry.path, { entry, sourceETag: entry.etag!, key: key(entry), visible: false, failed: false, retries: 0 })
    }
    pump()
  }
  function valid(demand: Demand, job: Job, epoch: number) {
    return enabled() && !job.controller.signal.aborted && epoch === scopeEpoch && demands.get(demand.entry.path) === demand && demand.visible && demand.sourceETag === demand.entry.etag
  }
  async function load(demand: Demand, job: Job, epoch: number) {
    try {
      const query = new URLSearchParams({ agentSpaceId: options.spaceId(), path: demand.entry.path, expectedEtag: demand.sourceETag, size: String(size()) })
      const response = await apiResponse(`${options.baseUrl()}/thumbnail?${query}`, { signal: job.controller.signal })
      if (!valid(demand, job, epoch)) { await response.body?.cancel(); return }
      if (response.status !== 200 || response.headers.get('Content-Type')?.split(';')[0]?.trim() !== 'image/webp' || response.headers.get('Agent-Space-Id') !== options.spaceId() || response.headers.get('Preview-Source-ETag') !== demand.sourceETag) {
        await response.body?.cancel(); throw new Error('Invalid thumbnail response')
      }
      const bytes = await boundedBytes(response, bodyLimit, job.controller.signal)
      if (!bytes.length) throw new Error('Empty thumbnail response')
      if (!valid(demand, job, epoch)) return
      urls.set(demand.key, URL.createObjectURL(new Blob([bytes as BlobPart], { type: 'image/webp' })))
    } catch (error) {
      if (valid(demand, job, epoch)) {
        if (error instanceof ApiProblem && error.status === 429) {
          concurrency = 1
          demand.failed = demand.retries++ >= 2
          cooldownUntil = Math.max(cooldownUntil, Date.now() + (error.retryAfterSeconds ?? 1) * 1000)
          resumeCooldown()
        } else demand.failed = true
      }
    } finally {
      if (demand.job === job) demand.job = undefined
      jobs.delete(job)
      pump()
    }
  }
  function pump() {
    if (!enabled() || cooldownUntil > Date.now() || !capability?.supported || !capability.sizes.includes(size())) return
    for (const demand of demands.values()) {
      if (jobs.size >= concurrency) break
      if (!demand.visible || demand.failed || demand.job || demand.sourceETag !== demand.entry.etag || urls.has(demand.key) || !capability.mediaKinds.includes(mediaKind(demand.entry)!)) continue
      const job = { controller: new AbortController() }
      demand.job = job; jobs.add(job)
      void load(demand, job, scopeEpoch)
    }
  }
  async function discover() {
    if (!enabled() || capabilityAttempted) return
    capabilityAttempted = true
    const epoch = scopeEpoch, controller = new AbortController()
    capabilityController = controller
    try {
      const query = new URLSearchParams({ agentSpaceId: options.spaceId() })
      const response = await apiResponse(`${options.baseUrl()}/thumbnail?${query}`, { method: 'OPTIONS', signal: controller.signal })
      if (controller.signal.aborted || epoch !== scopeEpoch) { await response.body?.cancel(); return }
      if (response.status !== 200 || response.headers.get('Content-Type')?.split(';')[0]?.trim() !== 'application/json' || response.headers.get('Agent-Space-Id') !== options.spaceId()) {
        await response.body?.cancel(); return
      }
      const value: unknown = JSON.parse(new TextDecoder().decode(await boundedBytes(response, 16 * 1024, controller.signal)))
      if (controller.signal.aborted || epoch !== scopeEpoch) return
      const candidate = value as Capabilities | null
      if (candidate?.schemaVersion !== 1 || typeof candidate.supported !== 'boolean' || candidate.contentType !== 'image/webp' || !Array.isArray(candidate.sizes) || !candidate.sizes.every(n => [128, 256, 512].includes(n)) || !Array.isArray(candidate.mediaKinds) || !candidate.mediaKinds.every(kind => kind === 'IMAGE' || kind === 'VIDEO')) return
      capability = candidate
      pump()
    } catch { /* Optional capability errors leave the original file list usable. */ }
    finally { if (capabilityController === controller) capabilityController = undefined }
  }
  function queueDiscovery() {
    if (discoveryQueued) return
    discoveryQueued = true
    queueMicrotask(() => { discoveryQueued = false; void discover() })
  }
  function resetScope() {
    scopeEpoch++
    stopCooldown(); cooldownUntil = 0; concurrency = 2
    capabilityController?.abort(); capabilityController = undefined
    capability = undefined; capabilityAttempted = false
    clearDirectory(); syncEntries()
    // Revoke synchronously, but only query the final target after Vue applies
    // all props in this turn. Demands stay synchronous for render-time refs.
    queueDiscovery()
  }
  function bind(entry: Entry, value: Element | ComponentPublicInstance | null) {
    if (!(value instanceof Element) || !mediaKind(entry) || !demands.has(entry.path)) return
    const old = targets.get(entry.path)
    if (old === value) return
    if (old) observer?.unobserve(old)
    targets.set(entry.path, value)
    observer?.observe(value)
  }
  function visibility() {
    if (document.hidden) {
      stopCooldown()
      if (capabilityController) { capabilityController.abort(); capabilityController = undefined; capabilityAttempted = false }
      for (const demand of demands.values()) { release(demand); demand.visible = false }
      observer?.disconnect()
    } else {
      resumeCooldown()
      for (const target of targets.values()) observer?.observe(target)
      queueDiscovery()
    }
  }
  watch([scope, options.available], resetScope, { flush: 'sync' })
  watch([options.entries, size], syncEntries, { flush: 'sync' })
  onMounted(() => {
    mounted = true
    if (typeof IntersectionObserver !== 'undefined') observer = new IntersectionObserver(entries => {
      for (const entry of entries) {
        const path = entry.target.getAttribute('data-thumbnail-path'), demand = path ? demands.get(path) : undefined
        if (!demand || targets.get(path!) !== entry.target) continue
        demand.visible = entry.isIntersecting && entry.intersectionRatio > 0 && !document.hidden
        if (!demand.visible) release(demand)
      }
      pump()
    })
    document.addEventListener('visibilitychange', visibility)
    resetScope()
  })
  onBeforeUnmount(() => {
    alive = false; scopeEpoch++; stopCooldown(); capabilityController?.abort(); clearDirectory()
    document.removeEventListener('visibilitychange', visibility)
  })
  function failed(entry: Entry, event: Event) {
    const demand = demands.get(entry.path)
    if (!demand || demand.key !== key(entry) || (event.target as HTMLImageElement).getAttribute('src') !== urls.get(demand.key)) return
    demand.failed = true; release(demand)
  }
  function url(entry: Entry) {
    const demand = demands.get(entry.path)
    return demand && demand.sourceETag === entry.etag ? urls.get(demand.key) : undefined
  }
  return { bind, clearDirectory, restoreDirectory: syncEntries, failed, url }
}

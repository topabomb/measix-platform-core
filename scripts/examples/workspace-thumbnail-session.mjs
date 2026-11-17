import { createHash } from 'node:crypto'

const sizes = [128, 256, 512]
const kinds = ['IMAGE', 'VIDEO']
const mime = response => response.headers.get('Content-Type')?.split(';')[0]?.trim().toLowerCase()
const strongETag = value => typeof value === 'string' && /^"[\x21\x23-\x7e\x80-\xff]*"$/.test(value)
const protocolError = () => new PreviewError(503, 'preview_protocol_error')

export class PreviewError extends Error {
  constructor(status, code, retryAfterSeconds) {
    super(code)
    this.name = 'PreviewError'
    this.status = status
    this.code = code
    this.retryAfterSeconds = retryAfterSeconds
  }
}

async function discard(response) { await response.body?.cancel().catch(() => {}) }

async function boundedBytes(response, limit, signal) {
  const declared = response.headers.get('Content-Length')
  if (declared !== null && (!/^\d+$/.test(declared) || !Number.isSafeInteger(Number(declared)) || Number(declared) > limit)) {
    await discard(response); throw protocolError()
  }
  const reader = response.body?.getReader()
  if (!reader) throw protocolError()
  const chunks = []
  let length = 0
  const cancel = () => { void reader.cancel().catch(() => {}) }
  signal.addEventListener('abort', cancel, { once: true })
  try {
    for (;;) {
      signal.throwIfAborted()
      const { done, value } = await reader.read()
      signal.throwIfAborted()
      if (done) break
      length += value.length
      if (length > limit) throw protocolError()
      chunks.push(value)
    }
    if (declared !== null && Number(declared) !== length) throw protocolError()
    return new Uint8Array(Buffer.concat(chunks, length))
  } finally {
    signal.removeEventListener('abort', cancel)
    await reader.cancel().catch(() => {})
  }
}

async function json(response, limit, signal) {
  const bytes = await boundedBytes(response, limit, signal)
  try { return JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes)) }
  catch { throw protocolError() }
}

async function requireSuccess(response, signal) {
  if (response.status === 200) return
  let code = 'preview_unavailable'
  if (mime(response) === 'application/problem+json') {
    const problem = await json(response, 16 * 1024, signal)
    if (typeof problem?.code === 'string' && /^[a-z_]{1,80}$/.test(problem.code)) code = problem.code
  } else await discard(response)
  const retry = response.headers.get('Retry-After')
  const seconds = response.status === 429 ? (/^[1-9]\d{0,3}$/.test(retry ?? '') && Number(retry) <= 3600 ? Number(retry) : 1) : undefined
  throw new PreviewError(response.status, code, seconds)
}

// A cell may stop waiting without cancelling the connection's shared OPTIONS.
function waitForDiscovery(promise, signal) {
  return new Promise((resolve, reject) => {
    const abort = () => { signal.removeEventListener('abort', abort); reject(signal.reason) }
    signal.addEventListener('abort', abort, { once: true })
    promise.then(value => { signal.removeEventListener('abort', abort); resolve(value) }, error => { signal.removeEventListener('abort', abort); reject(error) })
    if (signal.aborted) abort()
  })
}

// One instance belongs to one authenticated connection/space/binding revision.
// The caller supplies its existing authenticated fetch and visible-read queue.
export class WorkspaceThumbnailSession {
  #endpoint
  #space
  #fetch
  #controller = new AbortController()
  #capability

  constructor({ endpoint, agentSpaceId, fetch = globalThis.fetch }) {
    const url = new URL(endpoint)
    if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password || url.search || url.hash || !url.pathname.endsWith('/workspace/thumbnail') || typeof agentSpaceId !== 'string' || !agentSpaceId.trim() || typeof fetch !== 'function') throw new TypeError('Expected a fixed Core thumbnail endpoint, space and authenticated fetch')
    this.#endpoint = url.href
    this.#space = agentSpaceId
    this.#fetch = fetch
  }

  #request(method, parameters, signal) {
    const url = new URL(this.#endpoint)
    url.searchParams.set('agentSpaceId', this.#space)
    for (const [name, value] of Object.entries(parameters)) url.searchParams.set(name, String(value))
    return this.#fetch(url.href, { method, signal, redirect: 'error', cache: 'no-store' })
  }

  capabilities() {
    if (this.#controller.signal.aborted) return Promise.reject(this.#controller.signal.reason)
    // Cache the failure too: a failed service is not "unsupported", and a
    // directory of images must not turn one failure into one probe per cell.
    return this.#capability ??= this.#discover()
  }

  async #discover() {
    const signal = this.#controller.signal
    const response = await this.#request('OPTIONS', {}, signal)
    await requireSuccess(response, signal)
    if (mime(response) !== 'application/json' || response.headers.get('Agent-Space-Id') !== this.#space) {
      await discard(response); throw protocolError()
    }
    const value = await json(response, 16 * 1024, signal)
    const sameSet = (actual, expected) => Array.isArray(actual) && actual.length === expected.length && new Set(actual).size === expected.length && actual.every(item => expected.includes(item))
    if (value?.schemaVersion !== 1 || typeof value.supported !== 'boolean' || value.contentType !== 'image/webp' || !sameSet(value.sizes, value.supported ? sizes : []) || !sameSet(value.mediaKinds, value.supported ? kinds : [])) throw protocolError()
    return Object.freeze({ schemaVersion: 1, supported: value.supported, sizes: Object.freeze(value.supported ? [...sizes] : []), contentType: 'image/webp', mediaKinds: Object.freeze(value.supported ? [...kinds] : []) })
  }

  async thumbnail({ path, expectedEtag, size = 128 }, { signal } = {}) {
    const active = signal ? AbortSignal.any([this.#controller.signal, signal]) : this.#controller.signal
    active.throwIfAborted()
    if (typeof path !== 'string' || !path || path.startsWith('/') || path.split('/').some(part => !part || part === '.' || part === '..') || !strongETag(expectedEtag) || !sizes.includes(size)) throw new TypeError('Expected a relative file path, strong source ETag and supported size')
    const capability = await waitForDiscovery(this.capabilities(), active)
    active.throwIfAborted()
    if (!capability.supported) return undefined
    const response = await this.#request('GET', { path, expectedEtag, size }, active)
    await requireSuccess(response, active)
    if (mime(response) !== 'image/webp' || response.headers.get('Agent-Space-Id') !== this.#space || response.headers.get('Preview-Source-ETag') !== expectedEtag || response.headers.get('Content-Length') === null) {
      await discard(response); throw protocolError()
    }
    const bytes = await boundedBytes(response, 1024 * 1024, active)
    const etag = '"' + createHash('sha256').update(bytes).digest('hex') + '"'
    if (bytes.length < 12 || Buffer.from(bytes.subarray(0, 4)).toString('ascii') !== 'RIFF' || Buffer.from(bytes.subarray(8, 12)).toString('ascii') !== 'WEBP' || response.headers.get('ETag') !== etag) throw protocolError()
    active.throwIfAborted()
    return { bytes, etag }
  }

  dispose() { this.#controller.abort() }
}

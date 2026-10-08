import { uid } from 'quasar'

export type CandidatePrefix = 'prv' | 'mdl' | 'img' | 'tts' | 'asr' | 'mcp' | 'rte' | 'asd' | 'str' | 'prc'

type UnauthorizedHandler = (() => void | Promise<void>) | undefined
let unauthorizedHandler: UnauthorizedHandler

export class ApiProblem extends Error {
  readonly currentConfigRevision?: number
  readonly currentSecretVersion?: number
  readonly currentPricingRevision?: number
  readonly retryAfterSeconds?: number
  readonly receivedMcpProtocolVersion?: string
  readonly supportedMcpProtocolVersions?: string[]

  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly activationId?: string,
    readonly currentDraftRevision?: number,
    extra?: Record<string, unknown>,
  ) {
    super(message)
    this.name = 'ApiProblem'
    if (extra) {
      if (typeof extra.currentConfigRevision === 'number') this.currentConfigRevision = extra.currentConfigRevision
      if (typeof extra.currentSecretVersion === 'number') this.currentSecretVersion = extra.currentSecretVersion
      if (typeof extra.currentPricingRevision === 'number') this.currentPricingRevision = extra.currentPricingRevision
      if (typeof extra.retryAfterSeconds === 'number') this.retryAfterSeconds = extra.retryAfterSeconds
      const isVersion = (value: unknown): value is string => typeof value === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(value)
      if (isVersion(extra.receivedMcpProtocolVersion)) this.receivedMcpProtocolVersion = extra.receivedMcpProtocolVersion
      if (Array.isArray(extra.supportedMcpProtocolVersions) && extra.supportedMcpProtocolVersions.length <= 16 && extra.supportedMcpProtocolVersions.every(isVersion)) this.supportedMcpProtocolVersions = [...extra.supportedMcpProtocolVersions]
    }
  }
}

// A transport or server failure cannot prove that a non-idempotent command
// rolled back. Callers must refresh state and require an explicit new action.
export function commandResultUncertain(cause: unknown): boolean {
  return !(cause instanceof ApiProblem) || cause.status >= 500
}

export function setUnauthorizedHandler(handler: UnauthorizedHandler) {
  unauthorizedHandler = handler
}

export function createCandidateId(prefix: CandidatePrefix): string {
  return `${prefix}_${uid()}`
}

export function createIdempotencyKey(): string {
  return `idem_${uid()}`
}

function isMutation(method?: string): boolean {
  const normalized = (method ?? 'GET').toUpperCase()
  return normalized !== 'GET' && normalized !== 'HEAD' && normalized !== 'OPTIONS'
}

export async function apiResponse(path: string, init: RequestInit = {}, csrfToken?: string): Promise<Response> {
  const headers = new Headers(init.headers)
  if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  if (csrfToken && isMutation(init.method)) headers.set('X-CSRF-Token', csrfToken)

  const response = await fetch(path, { ...init, headers, credentials: 'same-origin' })
  if (!response.ok) {
    let body: Record<string, unknown> = {}
    try {
      body = await response.json() as Record<string, unknown>
    } catch {
      // Stable HTTP status/code remain sufficient when an intermediary returned non-JSON.
    }
    if (response.status === 401 && unauthorizedHandler) await unauthorizedHandler()
    const retryAfter = Number(response.headers.get('Retry-After'))
    const extra = { ...body }
    if (Number.isFinite(retryAfter) && retryAfter > 0) extra.retryAfterSeconds = Math.ceil(retryAfter)
    throw new ApiProblem(
      response.status,
      String(body.code ?? 'http_error'),
      String(body.detail ?? body.title ?? response.statusText),
      typeof body.activationId === 'string' ? body.activationId : undefined,
      typeof body.currentDraftRevision === 'number' ? body.currentDraftRevision : undefined,
      extra,
    )
  }
  return response
}

export async function apiFetch<T>(path: string, init: RequestInit = {}, csrfToken?: string): Promise<T> {
  const response = await apiResponse(path, init, csrfToken)
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

export type CandidatePrefix = 'prv' | 'mdl' | 'tts' | 'asr' | 'mcp' | 'rte'

type UnauthorizedHandler = (() => void | Promise<void>) | undefined
let unauthorizedHandler: UnauthorizedHandler

export class ApiProblem extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly activationId?: string,
    readonly currentDraftRevision?: number,
  ) {
    super(message)
    this.name = 'ApiProblem'
  }
}

export function setUnauthorizedHandler(handler: UnauthorizedHandler) {
  unauthorizedHandler = handler
}

export function createCandidateId(prefix: CandidatePrefix): string {
  return `${prefix}_${crypto.randomUUID()}`
}

export function createIdempotencyKey(): string {
  return `idem_${crypto.randomUUID()}`
}

export function buildCursorQuery(values: Record<string, string | number | boolean | undefined>): string {
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(values)) {
    if (value !== undefined && value !== '') query.set(key, String(value))
  }
  const encoded = query.toString()
  return encoded ? `?${encoded}` : ''
}

function isMutation(method?: string): boolean {
  const normalized = (method ?? 'GET').toUpperCase()
  return normalized !== 'GET' && normalized !== 'HEAD' && normalized !== 'OPTIONS'
}

export async function apiFetch<T>(path: string, init: RequestInit = {}, csrfToken?: string): Promise<T> {
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
    throw new ApiProblem(
      response.status,
      String(body.code ?? 'http_error'),
      String(body.detail ?? body.title ?? response.statusText),
      typeof body.activationId === 'string' ? body.activationId : undefined,
      typeof body.currentDraftRevision === 'number' ? body.currentDraftRevision : undefined,
    )
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

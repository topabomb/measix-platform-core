export type PerformanceMedia = { path: string; kind: 'IMAGE' | 'VIDEO'; width?: number; height?: number; bytes: number; sha256: string }
export type PerformanceFixture = { schemaVersion: 1; directory: string; entries: PerformanceMedia[]; range: { path: string; bytes: number; start: number; length: number; sha256: string } }
export type PerformancePhase = { name: string; startedAt: number; endedAt: number; firstImageAt?: number; allVisibleAt?: number; visiblePaths: string[]; renderedPaths: string[]; sampleIntervalMs?: number; sampledAt?: number[] }
export type PerformanceRequest = { method: string; phase?: string; startedAt: number; endedAt?: number; status?: number; failed?: string; cancelled?: boolean }
type Counts = { total: number; success: number; busy: number; cancelled: number; failed: number; pending: number }
type Latency = { count: number; p50Ms: number | null; p95Ms: number | null }
export function performanceFixturePlan(): { path: string; width: number; height: number }[]
export function validatePerformanceFixture(value: unknown): PerformanceFixture
export function positiveTransferOverlap(range: { start: number; end: number }, transfers: readonly { start: number; end: number }[]): { overlappingThumbnailRequests: number; overlapMs: number }
type VisibleObservation = { visible: string[]; rendered: string[] }
type PhaseSample = () => Promise<VisibleObservation>
export function runSampledPhase(options: { phase: PerformancePhase; directory: string; expectedSize: 128 | 256; read: () => Promise<VisibleObservation & { directory: string; size: number }>; action: (sample: PhaseSample) => Promise<void>; settle: (sample: PhaseSample) => Promise<void>; now?: () => number; wait?: (ms: number, signal: AbortSignal) => Promise<void> }): Promise<void>
export function seedThumbnailPerformance<User extends { userId: string; spaceId: string }>(options: { out: string; mediaDirectory: string; users: User[]; put: (user: User, path: string, bytes: Buffer) => Promise<void> }): Promise<PerformanceFixture>
export function summarizePerformance(observations: { phases: PerformancePhase[]; requests: PerformanceRequest[] }): { requests: Counts; capabilityQueries: number; observedBrowserEventDuration: Latency; phases: { name: string; elapsedMs: number; firstImageMs: number | null; allVisibleMs: number | null; visible: number; rendered: number; coverage: number | null; requests: Counts; observedBrowserEventDuration: Latency }[] }

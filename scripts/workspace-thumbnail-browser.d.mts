type Timing = { start: number; end: number; abortAt?: number }
export function maximumConcurrentTransfers(transfers: readonly Timing[]): number
export function thumbnailReadInterval<T extends Timing>(value: T): T & { networkEnd: number }

// Playwright is owned by console/package.json; the root tooling package does
// not install it. This type-only path uses that existing dependency.
export function holdPreview(page: Pick<Page, 'route' | 'unroute'>, path: string, size: number | undefined, proofs: unknown[]): Promise<{ ready: Promise<void>; release(): Promise<void> }>
import type { Page } from '../console/node_modules/@playwright/test/index.js'

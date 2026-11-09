import type { components } from './generated'

type SystemStatus = components['schemas']['SystemStatus']

/** Bundle-hash comparison outcome. UNKNOWN means there is nothing to compare. */
export type BundleHashState = 'MATCHED' | 'MISMATCHED' | 'UNKNOWN'

export function hasPublishedConfiguration(status: SystemStatus | undefined): boolean {
  return !!status && status.activeManagedGeneration > 0 && status.desiredControlRevision > 0
}

/**
 * Both bundle hashes are optional in the contract. Report UNKNOWN when Hub
 * publishes no desired hash instead of asserting a divergence that cannot be
 * observed; the UI must distinguish "unknown" from "mismatched".
 */
export function bundleHashState(status: SystemStatus | undefined): BundleHashState {
  const desired = status?.desiredBundleHash
  const applied = status?.appliedBundleHash
  if (!desired) return 'UNKNOWN'
  if (!applied) return 'MISMATCHED'
  return desired === applied ? 'MATCHED' : 'MISMATCHED'
}

export function isManagedRuntimeConverged(status: SystemStatus | undefined): boolean {
  return !!status && hasPublishedConfiguration(status) && status.runtimeStatus === 'READY' &&
    status.relayReady && status.appliedControlRevision === status.desiredControlRevision &&
    bundleHashState(status) !== 'MISMATCHED'
}

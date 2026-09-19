import { describe, expect, it } from 'vitest'
import type { components } from './generated'
import { bundleHashState, hasPublishedConfiguration, isManagedRuntimeConverged } from './systemStatus'

type SystemStatus = components['schemas']['SystemStatus']

const hash = (fill: string) => `sha256:${fill.repeat(64)}`

const ready: SystemStatus = {
  buildVersion: 'test', dbHealth: 'OK', schemaIdentity: hash('a'),
  runtimeStatus: 'READY', activeManagedGeneration: 1, managedStateRevision: 1,
  desiredControlRevision: 1, appliedControlRevision: 1,
  desiredBundleHash: hash('b'),
  appliedBundleHash: hash('b'), relayReady: true,
}

describe('managed runtime convergence', () => {
  it('requires a published release and matching applied Relay state', () => {
    expect(isManagedRuntimeConverged(ready)).toBe(true)
    expect(hasPublishedConfiguration({ ...ready, activeManagedGeneration: 0, desiredControlRevision: 0 })).toBe(false)
    for (const invalid of [
      { activeManagedGeneration: 0, desiredControlRevision: 0 },
      { relayReady: false },
      { runtimeStatus: 'DEGRADED' as const },
      { appliedControlRevision: undefined },
      { appliedControlRevision: 0 },
      { appliedControlRevision: 2 },
      { appliedBundleHash: hash('c') },
      // Hub knows the desired bundle but Relay never reported applying it.
      { appliedBundleHash: undefined },
    ]) expect(isManagedRuntimeConverged({ ...ready, ...invalid })).toBe(false)
  })

  it('does not call an uncomparable bundle hash a divergence', () => {
    // Both hashes are optional in the contract. When Hub publishes no desired
    // hash there is nothing to compare, so revision equality stands; claiming
    // "not converged" here would be a false negative.
    expect(isManagedRuntimeConverged({ ...ready, desiredBundleHash: undefined, appliedBundleHash: undefined })).toBe(true)
    expect(bundleHashState({ ...ready, desiredBundleHash: undefined, appliedBundleHash: undefined })).toBe('UNKNOWN')
    expect(bundleHashState({ ...ready, desiredBundleHash: undefined })).toBe('UNKNOWN')
  })

  it('treats undefined status as not converged', () => {
    expect(isManagedRuntimeConverged(undefined)).toBe(false)
    expect(hasPublishedConfiguration(undefined)).toBe(false)
    expect(bundleHashState(undefined)).toBe('UNKNOWN')
  })
})

describe('bundle hash state', () => {
  it('separates matched, mismatched and unknown', () => {
    expect(bundleHashState(ready)).toBe('MATCHED')
    expect(bundleHashState({ ...ready, appliedBundleHash: hash('c') })).toBe('MISMATCHED')
    // Desired known, applied missing: Relay has not confirmed the bundle.
    expect(bundleHashState({ ...ready, appliedBundleHash: undefined })).toBe('MISMATCHED')
    expect(bundleHashState({ ...ready, desiredBundleHash: undefined })).toBe('UNKNOWN')
    expect(bundleHashState({ ...ready, desiredBundleHash: '', appliedBundleHash: '' })).toBe('UNKNOWN')
  })
})

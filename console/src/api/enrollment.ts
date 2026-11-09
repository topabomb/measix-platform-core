import type { components } from './generated'
import type { components as ClientComponents } from './generated-client'

type Grant = Pick<components['schemas']['CreateEnrollmentResponse'], 'code' | 'expiresAt'>
type Material = ClientComponents['schemas']['PlatformEnrollmentMaterial']

/** The same ephemeral document is used for both native QR scan and paste. */
export function encodeEnrollmentMaterial(platformOrigin: string, grant: Grant): string {
  const url = new URL(platformOrigin)
  if (!['http:', 'https:'].includes(url.protocol) || url.port === '0' || url.username || url.password || /[?#]/.test(platformOrigin) || url.pathname !== '/' || url.origin.length > 1024) {
    throw new Error('Invalid enrollment platform origin')
  }
  const timestamp = Date.parse(grant.expiresAt)
  if (!grant.code || grant.code.length > 128 || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(grant.expiresAt) || !Number.isFinite(timestamp) || new Date(timestamp).toISOString().slice(0, 19) !== grant.expiresAt.slice(0, 19)) {
    throw new Error('Invalid enrollment credential or expiry')
  }
  const material: Material = { formatVersion: 1, kind: 'PLATFORM_ENROLLMENT', platformUrl: url.origin, code: grant.code, expiresAt: grant.expiresAt }
  const encoded = JSON.stringify(material)
  if (new TextEncoder().encode(encoded).length > 2048) throw new Error('Enrollment material is too large')
  return encoded
}

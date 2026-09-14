import { describe, expect, it } from 'vitest'
import fixture from '../../../api/fixtures/enrollment/platform-v1.json'
import { encodeEnrollmentMaterial } from './enrollment'

describe('platform enrollment material', () => {
  it('encodes the public origin and credential in the canonical scan/paste envelope', () => {
    const value = encodeEnrollmentMaterial('https://platform.example/', fixture)
    expect(JSON.parse(value)).toEqual(fixture)
    expect(new TextEncoder().encode(value).length).toBeLessThanOrEqual(2048)
  })
  it.each(['http://platform.example', 'https://user:secret@platform.example', 'https://platform.example/internal', 'https://platform.example?code=secret', 'https://platform.example#fragment'])('rejects invalid public origin %s', (origin) => {
    expect(() => encodeEnrollmentMaterial(origin, fixture)).toThrow()
  })
  it('rejects malformed credentials and timestamps instead of generating a broken QR', () => {
    expect(() => encodeEnrollmentMaterial(fixture.platformUrl, { ...fixture, code: '' })).toThrow()
    expect(() => encodeEnrollmentMaterial(fixture.platformUrl, { ...fixture, code: 'a'.repeat(129) })).toThrow()
    expect(() => encodeEnrollmentMaterial(fixture.platformUrl, { ...fixture, expiresAt: 'tomorrow' })).toThrow()
  })
  it('requires explicit loopback HTTP support and never extends it to private hosts', () => {
    expect(() => encodeEnrollmentMaterial('http://localhost:8080', fixture)).toThrow()
    expect(JSON.parse(encodeEnrollmentMaterial('http://127.0.0.1:8080', fixture, true)).platformUrl).toBe('http://127.0.0.1:8080')
    expect(() => encodeEnrollmentMaterial('http://192.168.1.10', fixture, true)).toThrow()
  })
})

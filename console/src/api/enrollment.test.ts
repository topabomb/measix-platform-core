import { describe, expect, it } from 'vitest'
import fixture from '../../../api/fixtures/enrollment/platform-v1.json'
import { encodeEnrollmentMaterial } from './enrollment'

describe('platform enrollment material', () => {
  it('encodes the public origin and credential in the canonical scan/paste envelope', () => {
    const value = encodeEnrollmentMaterial('https://platform.example/', fixture)
    expect(JSON.parse(value)).toEqual(fixture)
    expect(new TextEncoder().encode(value).length).toBeLessThanOrEqual(2048)
  })
  it.each(['http://:9000', 'http://platform.example:0', 'http://platform.example:65536', 'https://platform.example?', 'https://platform.example#', 'ftp://platform.example', 'https://user:secret@platform.example', 'https://platform.example/internal', 'https://platform.example?code=secret', 'https://platform.example#fragment'])('rejects invalid public origin %s', (origin) => {
    expect(() => encodeEnrollmentMaterial(origin, fixture)).toThrow()
  })
  it('rejects malformed credentials and timestamps instead of generating a broken QR', () => {
    expect(() => encodeEnrollmentMaterial(fixture.platformUrl, { ...fixture, code: '' })).toThrow()
    expect(() => encodeEnrollmentMaterial(fixture.platformUrl, { ...fixture, code: 'a'.repeat(129) })).toThrow()
    expect(() => encodeEnrollmentMaterial(fixture.platformUrl, { ...fixture, expiresAt: 'tomorrow' })).toThrow()
  })
  it.each(['http://localhost:8080', 'http://192.168.1.10:9000', 'http://203.0.113.10', 'http://platform.example', 'http://[2001:db8::1]:9000'])('supports HTTP origin %s without a development bypass', (origin) => {
    expect(JSON.parse(encodeEnrollmentMaterial(origin, fixture)).platformUrl).toBe(origin)
  })
})

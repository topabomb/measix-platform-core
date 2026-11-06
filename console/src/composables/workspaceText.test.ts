import { describe, expect, it } from 'vitest'
import { decodeEditableText, encodeEditableText } from './workspacePreview'

describe('workspace text editing', () => {
  it('preserves a leading content character after the UTF-8 BOM', () => {
    const bytes = new TextEncoder().encode('\uFEFF\uFEFFtext\n')
    const doc = decodeEditableText(bytes)
    expect(doc.text).toBe('\uFEFFtext\n')
    expect(Array.from(encodeEditableText(doc.text, doc))).toEqual(Array.from(bytes))
  })
  it('round-trips UTF-8 BOM and CRLF while editing normalized text', () => {
    const bytes = new Uint8Array([239,187,191,...new TextEncoder().encode('中文\r\nsecond\r\n')])
    const doc = decodeEditableText(bytes)
    expect(doc.text).toBe('中文\nsecond\n')
    expect(Array.from(encodeEditableText(doc.text, doc))).toEqual(Array.from(bytes))
  })
  it('rejects binary, invalid UTF-8 and mixed newlines instead of rewriting them', () => {
    for (const bytes of [new Uint8Array([0xff]),new Uint8Array([0]),new TextEncoder().encode('a\r\nb\nc')]) {
      expect(() => decodeEditableText(bytes)).toThrow()
    }
  })
})

import { describe, expect, it } from 'vitest'
import { renderWorkspaceMarkdown, previewKind, decodeWorkspaceText, rasterMetadata } from './workspacePreview'

describe('workspace preview boundary', () => {
 it('does not execute uploaded HTML or SVG', () => {
  expect(previewKind('x.html')).toBe('download')
  expect(previewKind('x.svg')).toBe('download')
  expect(previewKind('x.pdf')).toBe('pdf')
 })
 it('blocks raw HTML, script links and external images', () => {
  const result = renderWorkspaceMarkdown('<script>alert(1)</script>\n[x](javascript:alert(1))\n![tracking](https://evil.invalid/a.png)\n![local](image.png)', 'docs/a.md')
  expect(result.html).not.toContain('<script>')
  expect(result.html).not.toContain('href="javascript:')
  expect(result.html).not.toContain('src="https://evil.invalid')
  expect(result.images.map(x => x.path)).toEqual(['docs/image.png'])
 })
 it('rejects invalid UTF-8 and binary text instead of guessing', () => {
  expect(() => decodeWorkspaceText(new Uint8Array([0xff]))).toThrow()
  expect(() => decodeWorkspaceText(new Uint8Array([0,1,2]))).toThrow()
 })
})

it('rejects oversized image metadata before decoding bitmap pixels',()=>{
 const png=new Uint8Array(33);png.set([137,80,78,71,13,10,26,10]);png.set([73,72,68,82],12);const view=new DataView(png.buffer);view.setUint32(16,100000);view.setUint32(20,100000)
 expect(()=>rasterMetadata(png)).toThrow(/像素/)
 view.setUint32(16,400);view.setUint32(20,200)
 expect(rasterMetadata(png)).toEqual({type:'image/png',width:400,height:200})
 expect(()=>rasterMetadata(new Uint8Array([137,80,78,71]))).toThrow()
})

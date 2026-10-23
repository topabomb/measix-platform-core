import { afterEach, expect, it, vi } from 'vitest'
import { cursorPath, fetchAllPages } from './pagination'

afterEach(() => vi.unstubAllGlobals())

it('loads all selector pages and preserves query filters', async () => {
  const fetch = vi.fn()
    .mockResolvedValueOnce(new Response(JSON.stringify({items:[1],nextCursor:'next+key'}),{status:200}))
    .mockResolvedValueOnce(new Response(JSON.stringify({items:[2]}),{status:200}))
  vi.stubGlobal('fetch',fetch)
  expect(await fetchAllPages<number>('/items?limit=200&status=ACTIVE')).toEqual([1,2])
  expect(fetch.mock.calls[1]?.[0]).toBe('/items?limit=200&status=ACTIVE&cursor=next%2Bkey')
  expect(cursorPath('/items?cursor=old','new')).toBe('/items?cursor=new')
})

it('rejects a repeated cursor rather than looping forever',async () => {
  vi.stubGlobal('fetch',vi.fn().mockImplementation(() => Promise.resolve(new Response(JSON.stringify({items:[],nextCursor:'again'}),{status:200}))))
  await expect(fetchAllPages('/items')).rejects.toThrow('repeated')
})

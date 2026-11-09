import { describe, expect, it } from 'vitest'
import { renderContent } from './useMarkdown'

function render(content: string, format = 'MARKDOWN') {
  const node = document.createElement('div')
  node.innerHTML = renderContent(content, format)
  return node
}

describe('enterprise update text rendering', () => {
  it('renders PLAIN and unknown formats literally, including HTML characters', () => {
    for (const format of ['PLAIN', 'unknown']) {
      const content = '<strong>literal</strong> & **text**\nnext line'
      const node = render(content, format)
      expect(node.textContent).toBe(content)
      expect(node.children).toHaveLength(0)
    }
  })

  it('does not interpret raw HTML or fetch embedded images', () => {
    const node = render('<b>raw HTML</b>\n\n![diagram](https://example.invalid/tracker.png)\n\n<img src="https://example.invalid/raw.png">')
    expect(node.querySelector('b, img')).toBeNull()
    expect(node.textContent).toContain('<b>raw HTML</b>')
    expect(node.textContent).toContain('diagram')
  })

  it('preserves supported Markdown while removing executable links', () => {
    const node = render('# Title\n\n**bold** and *italic* and `code`\n\n- item\n\n> quote\n\n[link](https://example.invalid) [bad](javascript:alert%281%29)\n\n```\n<b>code</b>\n```')
    for (const selector of ['h1', 'strong', 'em', 'code', 'ul li', 'blockquote', 'pre']) {
      expect(node.querySelector(selector)).not.toBeNull()
    }
    expect(node.querySelector('a[href="https://example.invalid"]')).not.toBeNull()
    expect(node.querySelector('a[href^="javascript:"]')).toBeNull()
    expect(node.querySelector('pre')?.textContent).toContain('<b>code</b>')
  })
})

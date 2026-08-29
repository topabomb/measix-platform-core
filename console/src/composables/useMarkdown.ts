import { marked } from 'marked'
import DOMPurify from 'dompurify'

marked.setOptions({
  breaks: true,
  gfm: true,
})

/**
 * Render Markdown content to sanitized HTML.
 * Falls through as plain text when contentFormat is not MARKDOWN.
 */
export function renderContent(content: string, contentFormat: string): string {
  if (contentFormat === 'MARKDOWN') {
    const raw = marked.parse(content, { async: false }) as string
    return DOMPurify.sanitize(raw)
  }
  // PLAIN: escape HTML to prevent XSS
  return DOMPurify.sanitize(content)
}

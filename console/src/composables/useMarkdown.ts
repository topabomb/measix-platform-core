import { Marked } from 'marked'
import DOMPurify from 'dompurify'

function escapeText(content: string): string {
  return content.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

// A private parser keeps the Enterprise Update subset independent of other views.
const markdown = new Marked({
  gfm: false,
  renderer: {
    html: ({ text }) => escapeText(text),
    image: ({ text }) => escapeText(text),
  },
})

/**
 * Render Markdown content to sanitized HTML.
 * Falls through as plain text when contentFormat is not MARKDOWN.
 */
export function renderContent(content: string, contentFormat: string): string {
  if (contentFormat === 'MARKDOWN') {
    const raw = markdown.parse(content, { async: false }) as string
    return DOMPurify.sanitize(raw, {
      ALLOWED_TAGS: ['h1', 'h2', 'h3', 'p', 'br', 'strong', 'em', 'ul', 'ol', 'li', 'pre', 'code', 'a', 'blockquote', 'hr'],
      ALLOWED_ATTR: ['href', 'title', 'start'],
      ALLOW_DATA_ATTR: false,
      ALLOW_ARIA_ATTR: false,
    })
  }
  // PLAIN: escape HTML to prevent XSS
  return escapeText(content)
}

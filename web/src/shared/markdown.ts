// Markdown rendering for agent messages. Agent output is untrusted text: it is
// parsed as GitHub-flavored markdown and then sanitized with DOMPurify, so
// common formatting renders while any live markup or event handler the backend
// sends is stripped before it reaches v-html. Code blocks are wrapped with a
// copy button in the top-right corner.
import DOMPurify from 'dompurify'
import { marked, Renderer } from 'marked'

const defaultRenderer = new Renderer()
const origCode = defaultRenderer.code.bind(defaultRenderer)

marked.use({
  gfm: true,
  breaks: true,
  renderer: {
    code(token) {
      const rawCodeHtml = origCode(token)
      return (
        '<div class="code-block-wrapper">' +
        '<button type="button" class="code-copy-btn" title="复制代码" aria-label="复制代码">' +
        '<svg class="code-copy-icon code-copy-icon-idle" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' +
        '<rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>' +
        '<path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>' +
        '</svg>' +
        '<svg class="code-copy-icon code-copy-icon-copied" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' +
        '<polyline points="20 6 9 17 4 12"></polyline>' +
        '</svg>' +
        '<span class="code-copy-text">复制</span>' +
        '<span class="code-copied-text">已复制</span>' +
        '</button>' +
        rawCodeHtml +
        '</div>'
      )
    },
  },
})

const MAX_CACHE_SIZE = 1000
const markdownCache = new Map<string, string>()

export function clearMarkdownCache(): void {
  markdownCache.clear()
}

export function renderMarkdown(source: string): string {
  if (!source) return ''
  const cached = markdownCache.get(source)
  if (cached !== undefined) {
    // Refresh LRU access order
    markdownCache.delete(source)
    markdownCache.set(source, cached)
    return cached
  }

  const html = marked.parse(source, { async: false }) as string
  const sanitized = DOMPurify.sanitize(html)

  if (markdownCache.size >= MAX_CACHE_SIZE) {
    const oldestKey = markdownCache.keys().next().value
    if (oldestKey !== undefined) {
      markdownCache.delete(oldestKey)
    }
  }
  markdownCache.set(source, sanitized)
  return sanitized
}

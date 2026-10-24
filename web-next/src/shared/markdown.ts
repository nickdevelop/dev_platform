// Markdown rendering for agent messages. Agent output is untrusted text: it is
// parsed as GitHub-flavored markdown and then sanitized with DOMPurify, so
// common formatting renders while any live markup or event handler the backend
// sends is stripped before it reaches v-html.
import DOMPurify from 'dompurify'
import { marked } from 'marked'

marked.use({ gfm: true, breaks: true })

export function renderMarkdown(source: string): string {
  const html = marked.parse(source, { async: false })
  return DOMPurify.sanitize(html)
}

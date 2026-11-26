// Query highlighting for the quick search results. Text is HTML-escaped
// first and only then the match spans are inserted, so the v-html output can
// never carry markup from workspace file names or content.

export function escapeHtml(str: string): string {
  return str.replace(/[&<>"']/g, (m) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[m]!))
}

// highlightMatch wraps every case-insensitive occurrence of the query in a
// .qs-hit span. The query is regex-escaped, so characters like "(" stay
// literal.
export function highlightMatch(text: string, query: string): string {
  const safe = escapeHtml(text)
  const q = query.trim()
  if (!q) return safe
  const escaped = q.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return safe.replace(
    new RegExp(`(${escaped})`, 'gi'),
    '<span class="qs-hit">$1</span>',
  )
}

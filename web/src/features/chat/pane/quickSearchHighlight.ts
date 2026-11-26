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

// clipAroundQuery 以命中位置为中心截取一段摘要：命中点前后各保留 radius 个
// 字符，两端被裁时补省略号；无查询或未命中时从头截取，避免长行撞爆结果行。
export function clipAroundQuery(content: string, query: string, radius = 40): string {
  const text = content.trim()
  const span = Math.max(1, radius) * 2
  const q = query.trim().toLowerCase()
  const idx = q ? text.toLowerCase().indexOf(q) : -1
  if (idx < 0) {
    return text.length <= span ? text : `${text.slice(0, span)}…`
  }
  const start = Math.max(0, idx - radius)
  const end = Math.min(text.length, idx + q.length + radius)
  let out = text.slice(start, end)
  if (start > 0) out = `…${out}`
  if (end < text.length) out = `${out}…`
  return out
}

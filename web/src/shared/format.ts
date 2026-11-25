// Shared text formatting: token counts and middle-ellipsis paths.
export function formatTokens(count: number): string {
  if (!Number.isFinite(count) || count < 0) return '0'
  if (count < 1000) return String(Math.round(count))
  if (count < 10000) return `${(count / 1000).toFixed(1)}k`
  if (count < 1000000) return `${Math.round(count / 1000)}k`
  const millions = count / 1000000
  return `${millions % 1 === 0 ? String(millions) : millions.toFixed(1)}M`
}

// ellipsisMiddle shortens a long path by keeping its head and tail:
// "/data/.../dev_platform". A short path is returned unchanged.
export function ellipsisMiddle(path: string, maxLength = 48): string {
  if (path.length <= maxLength) return path
  const keep = Math.max(8, Math.floor((maxLength - 5) / 2))
  const head = path.slice(0, Math.ceil(keep))
  const tail = path.slice(path.length - Math.floor(keep))
  return `${head}/.../${tail}`
}

/**
 * 列表时间列的统一写法：`YYYY-MM-DD HH:mm`。
 *
 * 不用 `toLocaleString('zh-CN')`：它靠运行时的 locale 数据，Node / 浏览器结果不一致，
 * 窄屏下还会折行成两行（见 OrgPromptsView 的同名注释）。也不用不带参的
 * `toLocaleString()`：它按 locale 换分隔符与顺序，列宽对不齐。
 */
export function formatListTime(isoString?: string): string {
  if (!isoString) return '-'
  const date = new Date(isoString)
  if (isNaN(date.getTime())) return isoString
  const pad = (n: number) => (n < 10 ? `0${n}` : `${n}`)
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ` +
    `${pad(date.getHours())}:${pad(date.getMinutes())}`
  )
}

export function formatMessageTime(isoString?: string): string {
  if (!isoString) return ''
  const date = new Date(isoString)
  if (isNaN(date.getTime())) return ''
  const pad = (n: number) => (n < 10 ? `0${n}` : `${n}`)
  const hours = pad(date.getHours())
  const minutes = pad(date.getMinutes())
  const now = new Date()
  const isSameDay =
    date.getFullYear() === now.getFullYear() &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate()
  if (isSameDay) {
    return `${hours}:${minutes}`
  }
  const month = pad(date.getMonth() + 1)
  const day = pad(date.getDate())
  return `${month}-${day} ${hours}:${minutes}`
}

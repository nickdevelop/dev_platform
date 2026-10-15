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

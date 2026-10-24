// Utility functions for resolving workspace-relative paths and media URLs.

export function getDirectory(filePath: string): string {
  const normalized = filePath.replace(/\\/g, '/')
  const lastSlash = normalized.lastIndexOf('/')
  if (lastSlash === -1) return ''
  return normalized.slice(0, lastSlash)
}

export function resolveRelativePath(baseDir: string, relPath: string): string {
  const normalized = relPath.replace(/\\/g, '/')
  if (normalized.startsWith('/')) {
    return normalized.replace(/^\/+/, '')
  }
  const parts = baseDir ? baseDir.split('/').filter(Boolean) : []
  const relParts = normalized.split('/').filter(Boolean)

  for (const part of relParts) {
    if (part === '.') continue
    if (part === '..') {
      if (parts.length > 0) {
        parts.pop()
      }
    } else {
      parts.push(part)
    }
  }
  return parts.join('/')
}

export function isExternalUrl(url: string): boolean {
  return /^(https?:\/\/|mailto:|data:|#)/i.test(url)
}

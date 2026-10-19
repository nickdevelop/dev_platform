/**
 * Copies the given text to the clipboard.
 * Uses navigator.clipboard if available, falling back to document.execCommand('copy').
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  if (!text) return false

  if (typeof navigator !== 'undefined' && navigator.clipboard && typeof navigator.clipboard.writeText === 'function') {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // Non-secure contexts or permission denied: fall back to execCommand
    }
  }

  if (typeof document !== 'undefined' && typeof document.createElement === 'function') {
    try {
      const area = document.createElement('textarea')
      area.value = text
      area.style.position = 'fixed'
      area.style.left = '-9999px'
      area.style.top = '0'
      area.setAttribute('readonly', '')
      document.body.appendChild(area)
      area.select()
      const successful = document.execCommand('copy')
      area.remove()
      return successful
    } catch {
      return false
    }
  }

  return false
}

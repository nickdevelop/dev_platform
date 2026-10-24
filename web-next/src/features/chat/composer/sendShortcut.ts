export type SendShortcut = 'enter' | 'ctrl-enter'

export const SEND_SHORTCUT_STORAGE_KEY = 'dev-platform-send-shortcut'

export function getPersistedSendShortcut(): SendShortcut {
  try {
    const val = localStorage.getItem(SEND_SHORTCUT_STORAGE_KEY)
    if (val === 'ctrl-enter') return 'ctrl-enter'
    return 'enter'
  } catch {
    return 'enter'
  }
}

export function setPersistedSendShortcut(shortcut: SendShortcut): void {
  try {
    localStorage.setItem(SEND_SHORTCUT_STORAGE_KEY, shortcut)
  } catch {
    // LocalStorage fallback
  }
}

export function shouldSubmitOnKeydown(event: KeyboardEvent, shortcut: SendShortcut): boolean {
  if (event.key !== 'Enter') return false

  if (shortcut === 'ctrl-enter') {
    return event.ctrlKey || event.metaKey
  }

  // default 'enter': Enter submits, Shift+Enter makes a new line
  return !event.shiftKey && !event.ctrlKey && !event.metaKey && !event.altKey
}

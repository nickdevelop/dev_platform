import type { BrowserWindow, Input } from 'electron'

export interface InputEventLike {
  type: string
  key: string
  control?: boolean
  meta?: boolean
  shift?: boolean
  alt?: boolean
}

export interface ShortcutAction {
  isReload: boolean
  isForceReload: boolean
}

/**
 * Evaluates whether an input event corresponds to a reload shortcut.
 * Supports:
 * - Ctrl+R / Cmd+R -> Reload
 * - Ctrl+Shift+R / Cmd+Shift+R -> Force reload
 * - F5 -> Reload
 * - Ctrl+F5 / Shift+F5 -> Force reload
 */
export function matchReloadShortcut(input: InputEventLike): ShortcutAction {
  if (input.type !== 'keyDown') {
    return { isReload: false, isForceReload: false }
  }

  const key = (input.key || '').toLowerCase()
  const isCtrlOrMeta = Boolean(input.control || input.meta)
  const isShift = Boolean(input.shift)

  // Ctrl+R or Cmd+R
  if (isCtrlOrMeta && key === 'r') {
    return {
      isReload: !isShift,
      isForceReload: isShift,
    }
  }

  // F5 key
  if (key === 'f5') {
    const isForced = isShift || Boolean(input.control)
    return {
      isReload: !isForced,
      isForceReload: isForced,
    }
  }

  return { isReload: false, isForceReload: false }
}

/**
 * Registers window-scoped keyboard shortcuts via webContents before-input-event.
 * This ensures shortcuts function properly even when application/window menus are removed.
 */
export function registerWindowShortcuts(window: BrowserWindow): void {
  window.webContents.on('before-input-event', (event, input: Input) => {
    const action = matchReloadShortcut(input)
    if (action.isForceReload) {
      event.preventDefault()
      window.webContents.reloadIgnoringCache()
    } else if (action.isReload) {
      event.preventDefault()
      window.webContents.reload()
    }
  })
}

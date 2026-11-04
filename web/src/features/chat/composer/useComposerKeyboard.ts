import { ref, computed } from 'vue'
import {
  type SendShortcut,
  getPersistedSendShortcut,
  setPersistedSendShortcut,
  shouldSubmitOnKeydown,
} from './sendShortcut'

export function useComposerKeyboard(onSubmit?: () => void) {
  const sendShortcut = ref<SendShortcut>(getPersistedSendShortcut())
  const sendMenuOpen = ref(false)

  function toggleSendMenu() {
    sendMenuOpen.value = !sendMenuOpen.value
  }

  function setShortcut(shortcut: SendShortcut) {
    sendShortcut.value = shortcut
    setPersistedSendShortcut(shortcut)
    sendMenuOpen.value = false
  }

  function onWindowClick(event: MouseEvent) {
    if (!sendMenuOpen.value) return
    const target = event.target as Element | null
    if (target && !target.closest('.send-split-group')) {
      sendMenuOpen.value = false
    }
  }

  const sendShortcutHint = computed(() => {
    return sendShortcut.value === 'ctrl-enter'
      ? 'Ctrl+Enter 发送 / Enter 换行'
      : 'Enter 发送 / Shift+Enter 换行'
  })

  function handleKeydown(
    event: KeyboardEvent,
    isInterceptedByDropdown: boolean = false,
  ) {
    if (isInterceptedByDropdown) return
    if (shouldSubmitOnKeydown(event, sendShortcut.value)) {
      event.preventDefault()
      if (onSubmit) {
        onSubmit()
      }
    }
  }

  return {
    sendShortcut,
    sendMenuOpen,
    sendShortcutHint,
    toggleSendMenu,
    setShortcut,
    onWindowClick,
    handleKeydown,
  }
}

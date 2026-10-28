import { ref, type Ref } from 'vue'
import type { DropdownItem, DropdownSource, TriggerMatch } from './types'

export interface UseComposerDropdownOptions {
  sources: DropdownSource[]
  text: Ref<string>
  onApplyText: (newText: string, newCursor: number) => void
}

export function useComposerDropdown(options: UseComposerDropdownOptions) {
  const isOpen = ref(false)
  const items = ref<readonly DropdownItem[]>([])
  const selectedIndex = ref(0)
  const loading = ref(false)

  const activeMatch = ref<TriggerMatch | null>(null)
  const activeSource = ref<DropdownSource | null>(null)
  let itemSourceMap = new Map<string, DropdownSource>()

  let abortController: AbortController | null = null
  let isComposing = false

  function onCompositionStart() {
    isComposing = true
  }

  function onCompositionEnd(textarea: HTMLTextAreaElement) {
    isComposing = false
    return onInput(textarea)
  }

  async function onInput(textarea: HTMLTextAreaElement) {
    if (isComposing) return

    const cursor = textarea.selectionStart ?? 0
    const textVal = textarea.value

    const matched: Array<{ source: DropdownSource; match: TriggerMatch }> = []
    let primaryTrigger: string | null = null

    for (const source of options.sources) {
      const match = source.match(textVal, cursor)
      if (match) {
        if (primaryTrigger === null) {
          primaryTrigger = match.trigger
        }
        if (match.trigger === primaryTrigger) {
          matched.push({ source, match })
        }
      }
    }

    if (matched.length === 0) {
      close()
      return
    }

    activeSource.value = matched[0].source
    activeMatch.value = matched[0].match

    abortController?.abort()
    const ac = new AbortController()
    abortController = ac

    loading.value = true
    try {
      const results = await Promise.all(
        matched.map(async ({ source, match }) => {
          try {
            const loaded = await source.load(match, ac.signal)
            return { source, items: loaded }
          } catch {
            return { source, items: [] as readonly DropdownItem[] }
          }
        }),
      )
      if (ac.signal.aborted) return

      const combined: DropdownItem[] = []
      const map = new Map<string, DropdownSource>()

      for (const res of results) {
        for (const item of res.items) {
          if (!map.has(item.id)) {
            map.set(item.id, res.source)
            combined.push(item)
          }
        }
      }

      itemSourceMap = map
      items.value = combined
      selectedIndex.value = 0
      isOpen.value = combined.length > 0
    } catch {
      if (!ac.signal.aborted) {
        items.value = []
        isOpen.value = false
      }
    } finally {
      if (!ac.signal.aborted) {
        loading.value = false
      }
    }
  }

  function close() {
    abortController?.abort()
    abortController = null
    isOpen.value = false
    items.value = []
    itemSourceMap = new Map()
    selectedIndex.value = 0
    activeMatch.value = null
    activeSource.value = null
    loading.value = false
  }

  function selectItem(index: number) {
    const item = items.value[index]
    const match = activeMatch.value
    if (!item || !match) return

    const source = itemSourceMap.get(item.id) || activeSource.value
    if (!source) return

    const replacement = source.select(item, match)
    const currentText = options.text.value
    const before = currentText.slice(0, match.start)
    const after = currentText.slice(match.end)

    const insertText = typeof replacement === 'string' ? replacement : item.replacement
    const newText = before + insertText + after
    const newCursor = before.length + insertText.length

    close()
    options.onApplyText(newText, newCursor)

    if (item.action) {
      void Promise.resolve(item.action()).catch(() => undefined)
    }
  }

  function onKeydown(event: KeyboardEvent): boolean {
    if (!isOpen.value || items.value.length === 0) return false

    if (event.key === 'ArrowDown') {
      event.preventDefault()
      selectedIndex.value = (selectedIndex.value + 1) % items.value.length
      return true
    }

    if (event.key === 'ArrowUp') {
      event.preventDefault()
      selectedIndex.value = (selectedIndex.value - 1 + items.value.length) % items.value.length
      return true
    }

    if (event.key === 'Enter' || event.key === 'Tab') {
      event.preventDefault()
      selectItem(selectedIndex.value)
      return true
    }

    if (event.key === 'Escape') {
      event.preventDefault()
      close()
      return true
    }

    return false
  }

  return {
    isOpen,
    items,
    selectedIndex,
    loading,
    onInput,
    onKeydown,
    onCompositionStart,
    onCompositionEnd,
    selectItem,
    close,
  }
}

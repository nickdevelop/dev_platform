import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'
import { ProviderSlashSource } from '../features/chat/composer/dropdown/sources/ProviderSlashSource'
import { useComposerDropdown } from '../features/chat/composer/dropdown/useComposerDropdown'
import type { DropdownSource, DropdownItem } from '../features/chat/composer/dropdown/types'
import ComposerDropdown from '../features/chat/composer/dropdown/ComposerDropdown.vue'

describe('ProviderSlashSource (Platform Slash Commands)', () => {
  const source = new ProviderSlashSource(
    () => 0,
    () => '',
  )

  it('matches only at line start without spaces in query', () => {
    expect(source.match('/c', 2)).toEqual({
      trigger: '/',
      query: 'c',
      start: 0,
      end: 2,
      atLineStart: true,
    })

    // After space, no longer matches
    expect(source.match('/compact ', 9)).toBeNull()

    // Multiline line start matches
    expect(source.match('first line\n/co', 14)).toEqual({
      trigger: '/',
      query: 'co',
      start: 11,
      end: 14,
      atLineStart: true,
    })

    // Mid-line slash does not match
    expect(source.match('look at api/v1', 14)).toBeNull()
  })

  it('loads default platform commands (compact)', async () => {
    const all = await source.load(
      { trigger: '/', query: '', start: 0, end: 1, atLineStart: true },
      new AbortController().signal,
    )
    expect(all.length).toBe(1)
    expect(all[0].label).toBe('/compact')
    expect(all[0].badge).toBe('功能')

    const filtered = await source.load(
      { trigger: '/', query: 'comp', start: 0, end: 5, atLineStart: true },
      new AbortController().signal,
    )
    expect(filtered.some((item) => item.label === '/compact')).toBe(true)
  })
})

describe('useComposerDropdown', () => {
  it('handles input, keyboard navigation, and selection', async () => {
    const source = new ProviderSlashSource(() => 0, () => '')
    const text = ref('/c')
    let appliedText = ''
    let appliedCursor = 0

    const dropdown = useComposerDropdown({
      sources: [source],
      text,
      onApplyText: (newText, newCursor) => {
        appliedText = newText
        appliedCursor = newCursor
      },
    })

    const mockTextarea = {
      value: '/c',
      selectionStart: 2,
    } as unknown as HTMLTextAreaElement

    await dropdown.onInput(mockTextarea)
    expect(dropdown.isOpen.value).toBe(true)
    expect(dropdown.items.value.length).toBe(1)
    expect(dropdown.selectedIndex.value).toBe(0)

    // Keyboard navigation
    const downEvent = new KeyboardEvent('keydown', { key: 'ArrowDown' })
    const handledDown = dropdown.onKeydown(downEvent)
    expect(handledDown).toBe(true)
    expect(dropdown.selectedIndex.value).toBe(0)

    // Select with Enter
    const enterEvent = new KeyboardEvent('keydown', { key: 'Enter' })
    const handledEnter = dropdown.onKeydown(enterEvent)
    expect(handledEnter).toBe(true)
    expect(dropdown.isOpen.value).toBe(false)
    expect(appliedText).toBe('/compact ')
    expect(appliedCursor).toBe(appliedText.length)
  })

  it('suspends autocomplete during IME composition', async () => {
    const source = new ProviderSlashSource(() => 0, () => '')
    const text = ref('/')
    const dropdown = useComposerDropdown({
      sources: [source],
      text,
      onApplyText: () => {},
    })

    const mockTextarea = {
      value: '/',
      selectionStart: 1,
    } as unknown as HTMLTextAreaElement

    dropdown.onCompositionStart()
    await dropdown.onInput(mockTextarea)
    // While composing, dropdown should not open
    expect(dropdown.isOpen.value).toBe(false)

    await dropdown.onCompositionEnd(mockTextarea)
    // After composition ends, input runs
    expect(dropdown.isOpen.value).toBe(true)
  })

  it('aggregates multiple sources matching the same trigger', async () => {
    const platform = new ProviderSlashSource(() => 0, () => '')
    const customPromptSource: DropdownSource = {
      id: 'mock-prompts',
      trigger: '/' as const,
      match: (text: string, cursor: number) => platform.match(text, cursor),
      load: async () => [
        { id: 'platform:prompt:review', label: '/review', detail: '审查代码', kind: 'command', badge: '提示词', replacement: '/review ' },
      ],
      select: (item: DropdownItem) => item.replacement,
    }

    const text = ref('/')
    const dropdown = useComposerDropdown({
      sources: [platform, customPromptSource],
      text,
      onApplyText: () => {},
    })

    const mockTextarea = {
      value: '/',
      selectionStart: 1,
    } as unknown as HTMLTextAreaElement

    await dropdown.onInput(mockTextarea)
    expect(dropdown.isOpen.value).toBe(true)

    const labels = dropdown.items.value.map((item) => item.label)
    expect(labels).toContain('/compact')
    expect(labels).toContain('/review')
  })
})

describe('ComposerDropdown component', () => {
  it('renders dropdown items and emits select', async () => {
    const items = [
      { id: '1', label: '/compact', detail: '压缩会话上下文历史', kind: 'command' as const, replacement: '/compact ', badge: '功能' },
    ]

    const wrapper = mount(ComposerDropdown, {
      props: {
        items,
        selectedIndex: 0,
      },
    })

    const renderedItems = wrapper.findAll('.dropdown-item')
    expect(renderedItems.length).toBe(1)
    expect(renderedItems[0].classes()).toContain('active')

    await renderedItems[0].trigger('mousedown')
    expect(wrapper.emitted('select')?.[0]).toEqual([0])
  })
})

import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'
import { detectBuiltInCommand, BUILT_IN_COMMANDS } from '../features/chat/composer/builtInCommands'
import { BuiltInSlashSource } from '../features/chat/composer/dropdown/sources/BuiltInSlashSource'
import { useComposerDropdown } from '../features/chat/composer/dropdown/useComposerDropdown'
import ComposerDropdown from '../features/chat/composer/dropdown/ComposerDropdown.vue'

describe('builtInCommands', () => {
  it('detects /new and its aliases', () => {
    const res1 = detectBuiltInCommand('/new')
    expect(res1?.command.action).toBe('new')
    expect(res1?.args).toBe('')

    const res2 = detectBuiltInCommand('/clear')
    expect(res2?.command.action).toBe('new')
  })

  it('detects /model with args', () => {
    const res = detectBuiltInCommand('/model gpt-4o-mini')
    expect(res?.command.action).toBe('model')
    expect(res?.args).toBe('gpt-4o-mini')
  })

  it('detects /stop and /help', () => {
    expect(detectBuiltInCommand('/stop')?.command.action).toBe('stop')
    expect(detectBuiltInCommand('/help')?.command.action).toBe('help')
  })

  it('returns null for non-commands or unknown commands', () => {
    expect(detectBuiltInCommand('hello world')).toBeNull()
    expect(detectBuiltInCommand('/unknown-xyz')).toBeNull()
    expect(detectBuiltInCommand('')).toBeNull()
  })
})

describe('BuiltInSlashSource', () => {
  const source = new BuiltInSlashSource()

  it('matches only at line start without spaces in query', () => {
    expect(source.match('/m', 2)).toEqual({
      trigger: '/',
      query: 'm',
      start: 0,
      end: 2,
      atLineStart: true,
    })

    // After space, no longer matches
    expect(source.match('/model ', 7)).toBeNull()

    // Multiline line start matches
    expect(source.match('first line\n/st', 14)).toEqual({
      trigger: '/',
      query: 'st',
      start: 11,
      end: 14,
      atLineStart: true,
    })

    // Mid-line slash does not match
    expect(source.match('look at api/v1', 14)).toBeNull()
  })

  it('loads and filters built-in commands', () => {
    const all = source.load({ trigger: '/', query: '', start: 0, end: 1, atLineStart: true }, new AbortController().signal)
    expect(all.length).toBe(BUILT_IN_COMMANDS.length)

    const filtered = source.load({ trigger: '/', query: 'mod', start: 0, end: 4, atLineStart: true }, new AbortController().signal)
    expect(filtered.some((item) => item.label === '/model')).toBe(true)
  })
})

describe('useComposerDropdown', () => {
  it('handles input, keyboard navigation, and selection', async () => {
    const source = new BuiltInSlashSource()
    const text = ref('/m')
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
      value: '/m',
      selectionStart: 2,
    } as unknown as HTMLTextAreaElement

    await dropdown.onInput(mockTextarea)
    expect(dropdown.isOpen.value).toBe(true)
    expect(dropdown.items.value.length).toBeGreaterThan(0)
    expect(dropdown.selectedIndex.value).toBe(0)

    // Keyboard navigation
    const downEvent = new KeyboardEvent('keydown', { key: 'ArrowDown' })
    const handledDown = dropdown.onKeydown(downEvent)
    expect(handledDown).toBe(true)
    expect(dropdown.selectedIndex.value).toBe(1 % dropdown.items.value.length)

    // Select with Enter
    const enterEvent = new KeyboardEvent('keydown', { key: 'Enter' })
    const handledEnter = dropdown.onKeydown(enterEvent)
    expect(handledEnter).toBe(true)
    expect(dropdown.isOpen.value).toBe(false)
    expect(appliedText.startsWith('/model ')).toBe(true)
    expect(appliedCursor).toBe(appliedText.length)
  })

  it('suspends autocomplete during IME composition', async () => {
    const source = new BuiltInSlashSource()
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
})

describe('ComposerDropdown component', () => {
  it('renders dropdown items and emits select', async () => {
    const items = [
      { id: '1', label: '/new', detail: '新建会话', kind: 'command' as const, replacement: '/new ', badge: '内置' },
      { id: '2', label: '/stop', detail: '停止任务', kind: 'command' as const, replacement: '/stop ', badge: '内置' },
    ]

    const wrapper = mount(ComposerDropdown, {
      props: {
        items,
        selectedIndex: 0,
      },
    })

    const renderedItems = wrapper.findAll('.dropdown-item')
    expect(renderedItems.length).toBe(2)
    expect(renderedItems[0].classes()).toContain('active')

    await renderedItems[1].trigger('mousedown')
    expect(wrapper.emitted('select')?.[0]).toEqual([1])
  })
})

// Unit tests for TerminalWorkspace.vue
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import TerminalWorkspace from '../features/terminal/TerminalWorkspace.vue'
import { useTerminalStore } from '../stores/terminal'

vi.mock('xterm', () => {
  class TerminalMock {
    cols = 80
    rows = 24
    open = vi.fn()
    loadAddon = vi.fn()
    onData = vi.fn()
    write = vi.fn()
    writeln = vi.fn()
    dispose = vi.fn()
    focus = vi.fn()
  }
  return {
    Terminal: TerminalMock,
  }
})

vi.mock('@xterm/addon-fit', () => {
  class FitAddonMock {
    fit = vi.fn()
  }
  return {
    FitAddon: FitAddonMock,
  }
})

describe('TerminalWorkspace Component', () => {
  beforeEach(() => {
    vi.restoreAllMocks()

    globalThis.ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    } as unknown as typeof ResizeObserver

    globalThis.WebSocket = class {
      static OPEN = 1
      static CONNECTING = 0
      static CLOSING = 2
      static CLOSED = 3
      readyState = 1
      send = vi.fn()
      close = vi.fn()
    } as unknown as typeof WebSocket
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  function setupComponent() {
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useTerminalStore()
    store.servers = [
      {
        id: 0,
        name: '本机 (Localhost)',
        host: 'localhost',
        port: 0,
        username: 'local',
        auth_type: 'local',
        is_local: true,
        has_secret: false,
      },
    ]

    const wrapper = mount(TerminalWorkspace, {
      global: { plugins: [pinia] },
    })

    return { wrapper, store }
  }

  it('renders empty state when no tabs are open', () => {
    const { wrapper } = setupComponent()
    expect(wrapper.find('.empty-terminal-state').exists()).toBe(true)
    expect(wrapper.find('.terminal-tab-strip').exists()).toBe(false)
    expect(wrapper.text()).toContain('终端控制台')
  })

  it('renders tab strip and panes when tabs are opened', async () => {
    const { wrapper, store } = setupComponent()

    store.openTerminal(0)
    await wrapper.vm.$nextTick()

    expect(wrapper.find('.empty-terminal-state').exists()).toBe(false)
    expect(wrapper.find('.terminal-tab-strip').exists()).toBe(true)
    expect(wrapper.findAll('.terminal-tab-item')).toHaveLength(1)
    expect(wrapper.text()).toContain('本机')
  })

  it('switches and closes tabs', async () => {
    const { wrapper, store } = setupComponent()

    const tab1 = store.openTerminal(0)
    const tab2 = store.openTerminal(0)
    await wrapper.vm.$nextTick()

    expect(wrapper.findAll('.terminal-tab-item')).toHaveLength(2)
    expect(store.activeTabId).toBe(tab2)

    // Click tab 1 to activate it
    const tabItems = wrapper.findAll('.terminal-tab-item')
    await tabItems[0].trigger('click')
    expect(store.activeTabId).toBe(tab1)

    // Close tab 1
    const closeBtn = tabItems[0].find('.tab-close')
    await closeBtn.trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.findAll('.terminal-tab-item')).toHaveLength(1)
    expect(store.activeTabId).toBe(tab2)
  })

  it('clicks plus button to open new tab', async () => {
    const { wrapper, store } = setupComponent()

    store.openTerminal(0)
    await wrapper.vm.$nextTick()

    const plusBtn = wrapper.find('.btn-new-tab')
    await plusBtn.trigger('click')
    await wrapper.vm.$nextTick()

    expect(store.tabs).toHaveLength(2)
    expect(wrapper.findAll('.terminal-tab-item')).toHaveLength(2)
  })

  it('preserves terminal pane components when visible prop changes', async () => {
    const { wrapper, store } = setupComponent()

    store.openTerminal(0)
    await wrapper.vm.$nextTick()

    expect(wrapper.find('.terminal-pane').exists()).toBe(true)

    // Simulate navigation away from terminal: visible set to false
    await wrapper.setProps({ visible: false })
    expect(wrapper.find('.terminal-pane').exists()).toBe(true)

    // Simulate returning to terminal: visible set back to true
    await wrapper.setProps({ visible: true })
    expect(wrapper.find('.terminal-pane').exists()).toBe(true)
  })
})

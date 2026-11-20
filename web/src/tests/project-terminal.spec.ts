import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ProjectTerminalModal from '../features/terminal/ProjectTerminalModal.vue'
import QuickAssistantLauncher from '../features/chat/quick/QuickAssistantLauncher.vue'
import { useProjectTerminalStore } from '../stores/projectTerminal'
import { useTerminalStore } from '../stores/terminal'
import { useSettingsStore } from '../stores/settings'

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
    options = { fontSize: 11.5 }
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

vi.mock('../api/terminal', () => {
  return {
    buildTerminalWSUrl: vi.fn(() => 'ws://localhost/api/terminal/ws/0'),
    getTerminalStatus: vi.fn(async () => ({ supported: true, os: 'linux' })),
    listTerminalServers: vi.fn(async () => [
      { id: 1, name: '测试服务器', host: '192.168.1.100', port: 22, username: 'root', auth_type: 'password', is_local: false, has_secret: true },
    ]),
    createTerminalServer: vi.fn(),
    updateTerminalServer: vi.fn(),
    deleteTerminalServer: vi.fn(),
    testServerConnection: vi.fn(async () => ({ ok: true })),
  }
})

describe('Project Terminal Feature', () => {
  beforeEach(() => {
    setActivePinia(createPinia())

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
    document.body.innerHTML = ''
    vi.restoreAllMocks()
  })

  it('opens floating window with default 800x500 dimensions and displays tab title', async () => {
    const store = useProjectTerminalStore()
    store.openTerminal('/workspace/demo', 'DemoApp')

    const wrapper = mount(ProjectTerminalModal, {
      attachTo: document.body,
    })

    const modal = document.querySelector('[data-test="project-terminal-modal"]')
    expect(modal).not.toBeNull()

    // No full-screen backdrop blocking the page
    expect(document.querySelector('.project-terminal-backdrop')).toBeNull()

    const windowEl = modal as HTMLElement
    expect(windowEl.style.width).toBe('800px')
    expect(windowEl.style.height).toBe('500px')
    expect(windowEl.textContent).toContain('DemoApp')

    // No maximize button
    expect(modal?.querySelector('[data-test="terminal-maximize-btn"]')).toBeNull()

    wrapper.unmount()
  })

  it('supports dragging via title bar without closing on Escape', async () => {
    const store = useProjectTerminalStore()
    store.openTerminal('/workspace/demo', 'DemoApp')

    const wrapper = mount(ProjectTerminalModal, {
      attachTo: document.body,
    })

    const titleBar = document.querySelector('[data-test="terminal-title-bar"]') as HTMLElement
    expect(titleBar).not.toBeNull()

    const windowEl = document.querySelector('[data-test="project-terminal-modal"]') as HTMLElement
    expect(windowEl).not.toBeNull()

    const initialLeft = parseFloat(windowEl.style.left || '0')
    const initialTop = parseFloat(windowEl.style.top || '0')

    // Simulate pointer drag
    titleBar.dispatchEvent(new PointerEvent('pointerdown', { button: 0, clientX: 100, clientY: 100 }))
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 150, clientY: 130 }))
    window.dispatchEvent(new PointerEvent('pointerup'))

    await wrapper.vm.$nextTick()
    const movedLeft = parseFloat(windowEl.style.left || '0')
    const movedTop = parseFloat(windowEl.style.top || '0')

    expect(movedLeft).toBe(initialLeft + 50)
    expect(movedTop).toBe(initialTop + 30)

    // Simulate Escape key press on window: should NOT close
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(store.isOpen).toBe(true)
    expect(store.isMinimized).toBe(false)

    wrapper.unmount()
  })

  it('minimizes into global quick toolbar as the first button when quick assistant is disabled', async () => {
    const store = useProjectTerminalStore()
    const settings = useSettingsStore()
    settings.quickAssistant.enabled = false

    store.openTerminal('/workspace/demo', 'DemoApp')

    const modalWrapper = mount(ProjectTerminalModal, {
      attachTo: document.body,
    })
    const toolbarWrapper = mount(QuickAssistantLauncher, {
      attachTo: document.body,
    })

    // Initially modal is open, toolbar is not visible (because assistant is disabled & terminal not minimized)
    expect(store.isMinimized).toBe(false)
    expect(toolbarWrapper.find('[data-test="quick-assistant-toolbar"]').exists()).toBe(false)

    // Click minimize button
    const minimizeBtn = document.querySelector('[data-test="terminal-minimize-btn"]') as HTMLElement
    expect(minimizeBtn).not.toBeNull()
    minimizeBtn.click()

    expect(store.isMinimized).toBe(true)
    await toolbarWrapper.vm.$nextTick()

    // Toolbar now appears and terminal dock button is the first button
    const toolbar = toolbarWrapper.find('[data-test="quick-assistant-toolbar"]')
    expect(toolbar.exists()).toBe(true)
    const buttons = toolbar.findAll('button')
    expect(buttons.length).toBe(1)
    expect(buttons[0].attributes('data-test')).toBe('dock-terminal-button')

    // Click the terminal button to restore
    await buttons[0].trigger('click')
    expect(store.isMinimized).toBe(false)

    modalWrapper.unmount()
    toolbarWrapper.unmount()
  })

  it('places terminal minimize button AFTER assistant buttons when quick assistant is enabled', async () => {
    const store = useProjectTerminalStore()
    const settings = useSettingsStore()
    settings.quickAssistant.enabled = true

    store.openTerminal('/workspace/demo', 'DemoApp')
    store.minimize()

    const toolbarWrapper = mount(QuickAssistantLauncher, {
      attachTo: document.body,
    })
    await toolbarWrapper.vm.$nextTick()

    const toolbar = toolbarWrapper.find('[data-test="quick-assistant-toolbar"]')
    expect(toolbar.exists()).toBe(true)
    const buttons = toolbar.findAll('button')
    // At least 2 buttons: assistant button first, terminal button after
    expect(buttons.length).toBeGreaterThanOrEqual(2)
    const lastButton = buttons[buttons.length - 1]
    expect(lastButton.attributes('data-test')).toBe('dock-terminal-button')

    toolbarWrapper.unmount()
  })

  it('exits terminal only when close button is clicked', async () => {
    const store = useProjectTerminalStore()
    store.openTerminal('/workspace/demo', 'DemoApp')

    const modalWrapper = mount(ProjectTerminalModal, {
      attachTo: document.body,
    })

    const closeBtn = document.querySelector('[data-test="terminal-close-btn"]') as HTMLElement
    expect(closeBtn).not.toBeNull()
    closeBtn.click()

    expect(store.isOpen).toBe(false)
    expect(store.tabs.length).toBe(0)

    modalWrapper.unmount()
  })

  it('defaults to multiple tabs in single main window, and supports tab detachment & attachment', async () => {
    const store = useProjectTerminalStore()
    const tabAId = store.openTerminal('/workspace/app-a', 'App A')
    const tabBId = store.openTerminal('/workspace/app-b', 'App B')

    // Default multi-tab: 1 window with 2 tabs
    expect(store.windows.length).toBe(1)
    expect(store.tabs.length).toBe(2)

    const wrapper = mount(ProjectTerminalModal, {
      attachTo: document.body,
    })

    // Only 1 modal window rendered with 2 tab items
    const windows = document.querySelectorAll('[data-test="project-terminal-modal"]')
    expect(windows.length).toBe(1)

    const tabItems = document.querySelectorAll('[data-test="modal-tab-item"]')
    expect(tabItems.length).toBe(2)

    // Detach Tab B into a standalone window
    const detachBtns = document.querySelectorAll('[data-test="detach-tab-btn"]')
    expect(detachBtns.length).toBe(2)
    ;(detachBtns[1] as HTMLElement).click()
    await wrapper.vm.$nextTick()

    // Now there are 2 windows (Main window with Tab A, and Detached window with Tab B)
    expect(store.windows.length).toBe(2)
    const windowsAfterDetach = document.querySelectorAll('[data-test="project-terminal-modal"]')
    expect(windowsAfterDetach.length).toBe(2)

    // The detached window has an attach button
    const attachBtn = document.querySelector('[data-test="terminal-attach-btn"]') as HTMLElement
    expect(attachBtn).not.toBeNull()

    // Click attach button -> Tab B merges back into Main window
    attachBtn.click()
    await wrapper.vm.$nextTick()

    expect(store.windows.length).toBe(1)
    expect(store.tabs.filter((t) => t.windowId === 'main').length).toBe(2)
    expect(document.querySelectorAll('[data-test="project-terminal-modal"]').length).toBe(1)

    wrapper.unmount()
  })

  it('supports Alt+` (toggleShortcut) to toggle terminal modal visibility (minimize and restore)', async () => {
    const store = useProjectTerminalStore()
    expect(store.windows.length).toBe(0)

    // Press Alt+` when empty -> opens default local terminal
    store.toggleShortcut()
    expect(store.windows.length).toBe(1)
    expect(store.isOpen).toBe(true)
    expect(store.isMinimized).toBe(false)

    // Press Alt+` when open -> minimizes
    store.toggleShortcut()
    expect(store.isMinimized).toBe(true)

    // Press Alt+` when minimized -> restores
    store.toggleShortcut()
    expect(store.isMinimized).toBe(false)
  })

  it('provides + button to open dropdown and create new local or remote server tabs', async () => {
    const store = useProjectTerminalStore()
    const termStore = useTerminalStore()
    termStore.servers = [
      { id: 0, name: '本机', host: 'localhost', port: 0, username: '', auth_type: 'local', is_local: true, has_secret: false },
      { id: 10, name: '远程测试节点', host: '10.0.0.1', port: 22, username: 'admin', auth_type: 'password', is_local: false, has_secret: true },
    ]

    store.openTerminal('/workspace/demo', 'DemoApp')

    const wrapper = mount(ProjectTerminalModal, {
      attachTo: document.body,
    })

    const newTabBtn = document.querySelector('[data-test="new-terminal-tab-btn"]') as HTMLElement
    expect(newTabBtn).not.toBeNull()

    // Click '+' button -> dropdown appears
    newTabBtn.click()
    await wrapper.vm.$nextTick()

    const dropdown = document.querySelector('[data-test="new-tab-dropdown"]')
    expect(dropdown).not.toBeNull()

    // Click remote server option
    const serverItem = document.querySelector('[data-test="menu-item-server"]') as HTMLElement
    expect(serverItem).not.toBeNull()
    expect(serverItem.textContent).toContain('远程测试节点')
    serverItem.click()
    await wrapper.vm.$nextTick()

    // Now main window has 2 tabs, with second tab connected to server 10
    expect(store.tabs.length).toBe(2)
    expect(store.tabs[1].serverId).toBe(10)
    expect(store.tabs[1].serverName).toBe('远程测试节点')

    wrapper.unmount()
  })
})

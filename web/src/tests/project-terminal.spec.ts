import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ProjectTerminalModal from '../features/terminal/ProjectTerminalModal.vue'
import QuickAssistantLauncher from '../features/chat/quick/QuickAssistantLauncher.vue'
import { useProjectTerminalStore } from '../stores/projectTerminal'
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

  it('opens floating window with default 800x500 dimensions, no full-screen backdrop, and displays title & cwd', async () => {
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
    expect(windowEl.textContent).toContain('终端 - DemoApp')
    expect(windowEl.textContent).toContain('/workspace/demo')

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
    expect(store.cwd).toBe('')

    modalWrapper.unmount()
  })

  it('supports opening multiple terminals across sessions with bring-to-front on click', async () => {
    const store = useProjectTerminalStore()
    const idA = store.openTerminal('/workspace/app-a', 'App A')
    const idB = store.openTerminal('/workspace/app-b', 'App B')

    expect(store.instances.length).toBe(2)

    const wrapper = mount(ProjectTerminalModal, {
      attachTo: document.body,
    })

    const windows = document.querySelectorAll('[data-test="project-terminal-modal"]')
    expect(windows.length).toBe(2)

    // Window B was opened second, so its zIndex should be higher than A
    const instA = store.instances.find((i) => i.id === idA)!
    const instB = store.instances.find((i) => i.id === idB)!
    expect(instB.zIndex).toBeGreaterThan(instA.zIndex)

    // Click Window A -> its zIndex should become higher than B (bring to front)
    const winAEl = document.querySelector(`[data-terminal-id="${idA}"]`) as HTMLElement
    winAEl.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }))
    expect(instA.zIndex).toBeGreaterThan(instB.zIndex)

    // Close Window A -> only Window B remains
    const closeBtnA = winAEl.querySelector('[data-test="terminal-close-btn"]') as HTMLElement
    closeBtnA.click()

    expect(store.instances.length).toBe(1)
    expect(store.instances[0].id).toBe(idB)

    wrapper.unmount()
  })
})

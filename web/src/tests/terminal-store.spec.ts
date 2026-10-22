// Tests for terminal store and tab management.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useTerminalStore } from '../stores/terminal'
import { useAuthStore } from '../stores/auth'
import { installTestCSRF } from './csrf-fixture'

type Call = { method: string; path: string; body: unknown; headers?: Record<string, string> }

function stubFetch(routes: Record<string, unknown> = {}) {
  const calls: Call[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      const method = init?.method ?? 'GET'
      const body = init?.body ? JSON.parse(String(init.body)) : undefined
      const headers = (init?.headers as Record<string, string>) ?? {}
      calls.push({ method, path: url.replace('/api', ''), body, headers })
      const key = `${method} ${url.replace('/api', '')}`
      if (key in routes) return { status: 200, json: async () => routes[key] } as Response
      return { status: 200, json: async () => ({}) } as Response
    }),
  )
  return calls
}

function initStore() {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().$patch({ authenticated: true })
  return useTerminalStore()
}

installTestCSRF('csrf-token')

describe('terminal store', () => {
  beforeEach(() => vi.restoreAllMocks())
  afterEach(() => vi.unstubAllGlobals())

  it('loads terminal status', async () => {
    stubFetch({
      'GET /terminal/status': { supported: true, os: 'linux' },
    })
    const store = initStore()
    await store.loadStatus()
    expect(store.status?.supported).toBe(true)
    expect(store.status?.os).toBe('linux')
    expect(store.isSupported).toBe(true)
  })

  it('loads servers and defaults activeServerId to first server', async () => {
    stubFetch({
      'GET /terminal/servers': [
        { id: 0, name: '本机 (Localhost)', host: 'localhost', port: 0, username: 'local', auth_type: 'local', is_local: true, has_secret: false },
        { id: 1, name: 'Dev Server', host: '192.168.1.10', port: 22, username: 'ubuntu', auth_type: 'password', is_local: false, has_secret: true },
      ],
    })
    const store = initStore()
    await store.loadServers()
    expect(store.servers).toHaveLength(2)
    expect(store.activeServerId).toBe(0)
    expect(store.activeServer?.name).toBe('本机 (Localhost)')
  })

  it('manages tabs (open, switch, close)', () => {
    const store = initStore()
    store.servers = [
      { id: 0, name: '本机 (Localhost)', host: 'localhost', port: 0, username: 'local', auth_type: 'local', is_local: true, has_secret: false },
      { id: 1, name: 'Dev Server', host: '192.168.1.10', port: 22, username: 'ubuntu', auth_type: 'password', is_local: false, has_secret: true },
    ]

    // Open first tab for localhost
    const tab1 = store.openTerminal(0)
    expect(store.tabs).toHaveLength(1)
    expect(store.activeTabId).toBe(tab1)
    expect(store.activeTab?.title).toContain('本机')

    // Open second tab for localhost
    const tab2 = store.openTerminal(0)
    expect(store.tabs).toHaveLength(2)
    expect(store.activeTabId).toBe(tab2)
    expect(store.activeTab?.title).toContain('#2')

    // Switch to tab1
    store.setActiveTab(tab1)
    expect(store.activeTabId).toBe(tab1)

    // Close tab1 -> should activate tab2
    store.closeTab(tab1)
    expect(store.tabs).toHaveLength(1)
    expect(store.activeTabId).toBe(tab2)

    // Close remaining tab -> activeTabId should be null
    store.closeTab(tab2)
    expect(store.tabs).toHaveLength(0)
    expect(store.activeTabId).toBeNull()
  })

  it('creates and deletes third-party server', async () => {
    stubFetch({
      'POST /terminal/servers': {
        id: 2,
        name: 'New Server',
        host: '10.0.0.5',
        port: 22,
        username: 'root',
        auth_type: 'password',
        is_local: false,
        has_secret: true,
      },
      'DELETE /terminal/servers/2': { ok: true },
    })
    const store = initStore()
    const created = await store.createServer({
      name: 'New Server',
      host: '10.0.0.5',
      port: 22,
      username: 'root',
      auth_type: 'password',
      secret: 'pwd',
    })
    expect(created.id).toBe(2)
    expect(store.servers).toContainEqual(created)

    await store.deleteServer(2)
    expect(store.servers.find((s) => s.id === 2)).toBeUndefined()
  })
})

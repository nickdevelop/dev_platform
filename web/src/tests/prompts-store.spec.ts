// Tests for prompts store CRUD operations.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { usePromptsStore } from '../stores/prompts'
import { useAuthStore } from '../stores/auth'

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
  useAuthStore().$patch({ authenticated: true, csrfToken: 'csrf-token' })
  return usePromptsStore()
}

describe('prompts store', () => {
  beforeEach(() => vi.restoreAllMocks())
  afterEach(() => vi.unstubAllGlobals())

  it('loads prompt list via loadPrompts', async () => {
    stubFetch({
      'GET /prompts': {
        prompts: [
          { id: 1, name: 'Prompt A', description: 'Desc A', content: 'Content A', createdAt: '', updatedAt: '' },
          { id: 2, name: 'Prompt B', description: 'Desc B', content: 'Content B', createdAt: '', updatedAt: '' },
        ],
      },
    })
    const store = initStore()
    await store.loadPrompts()
    expect(store.prompts).toHaveLength(2)
    expect(store.byId(1)?.name).toBe('Prompt A')
    expect(store.byId(2)?.name).toBe('Prompt B')
  })

  it('creates a new prompt and unshifts into list', async () => {
    const calls = stubFetch({
      'POST /prompts': {
        id: 3,
        name: 'New Prompt',
        description: 'New Desc',
        content: 'New {user_prompt}',
        createdAt: '',
        updatedAt: '',
      },
    })
    const store = initStore()
    const created = await store.createPrompt({
      name: 'New Prompt',
      description: 'New Desc',
      content: 'New {user_prompt}',
    })
    expect(created?.id).toBe(3)
    expect(store.prompts).toHaveLength(1)
    expect(store.prompts[0].name).toBe('New Prompt')
    expect(calls[0].headers?.['X-CSRF-Token']).toBe('csrf-token')
  })

  it('updates an existing prompt', async () => {
    const calls = stubFetch({
      'PUT /prompts/1': {
        id: 1,
        name: 'Updated Name',
        description: 'Updated Desc',
        content: 'Updated Content',
        createdAt: '',
        updatedAt: '',
      },
    })
    const store = initStore()
    store.prompts = [
      { id: 1, name: 'Old Name', description: 'Old Desc', content: 'Old Content', createdAt: '', updatedAt: '' },
    ]
    store.currentPrompt = store.prompts[0]

    const updated = await store.updatePrompt(1, { name: 'Updated Name' })
    expect(updated?.name).toBe('Updated Name')
    expect(store.prompts[0].name).toBe('Updated Name')
    expect(store.currentPrompt?.name).toBe('Updated Name')
    expect(calls[0].headers?.['X-CSRF-Token']).toBe('csrf-token')
  })

  it('deletes a prompt and removes from list and currentPrompt', async () => {
    const calls = stubFetch({
      'DELETE /prompts/1': { ok: true },
    })
    const store = initStore()
    store.prompts = [
      { id: 1, name: 'P1', description: '', content: '', createdAt: '', updatedAt: '' },
      { id: 2, name: 'P2', description: '', content: '', createdAt: '', updatedAt: '' },
    ]
    store.currentPrompt = store.prompts[0]

    await store.deletePrompt(1)
    expect(store.prompts).toHaveLength(1)
    expect(store.prompts[0].id).toBe(2)
    expect(store.currentPrompt).toBeNull()
    expect(calls[0].headers?.['X-CSRF-Token']).toBe('csrf-token')
  })
})

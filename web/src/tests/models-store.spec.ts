// Tests for the model management client/store: CRUD round trips, the
// fetch-candidates probe and the model test.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useModelsStore } from '../stores/models'
import { useAuthStore } from '../stores/auth'

type Call = { method: string; path: string; body: unknown }

function stubFetch(routes: Record<string, unknown> = {}) {
  const calls: Call[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      const method = init?.method ?? 'GET'
      const body = init?.body ? JSON.parse(String(init.body)) : undefined
      calls.push({ method, path: url.replace('/api', ''), body })
      const key = `${method} ${url.replace('/api', '')}`
      if (key in routes) return { status: 200, json: async () => routes[key] } as Response
      return { status: 200, json: async () => ({}) } as Response
    }),
  )
  return calls
}

function store() {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().$patch({ authenticated: true, csrfToken: 'csrf-1' })
  return useModelsStore()
}

beforeEach(() => vi.restoreAllMocks())
afterEach(() => vi.unstubAllGlobals())

describe('models store', () => {
  it('loads services from GET /model-services', async () => {
    stubFetch({
      'GET /model-services': {
        services: [{ id: 1, name: 'local', baseUrl: 'http://x/v1', hasKey: false, secretRef: '', enabled: true, models: [] }],
      },
    })
    const models = store()
    await models.load()
    expect(models.services).toHaveLength(1)
    expect(models.services[0].name).toBe('local')
  })

  it('creates and deletes a service', async () => {
    const calls = stubFetch()
    const models = store()
    await models.saveService({ name: 's', baseUrl: 'https://x/v1', apiKey: 'k', enabled: true })
    let call = calls.find((c) => c.method === 'POST' && c.path === '/model-services')
    expect(call?.body).toMatchObject({ name: 's', apiKey: 'k' })

    await models.saveService({ name: 's', baseUrl: 'https://x/v1', apiKey: '', enabled: true }, 3)
    call = calls.find((c) => c.method === 'PUT' && c.path === '/model-services/3')
    expect(call?.body).toMatchObject({ apiKey: '' })

    await models.removeService(3)
    expect(calls.some((c) => c.method === 'DELETE' && c.path === '/model-services/3')).toBe(true)
  })

  it('fetches candidate models and tests a model', async () => {
    const calls = stubFetch({
      'POST /model-services/fetch-models': { models: [{ id: 'm1' }, { id: 'm2' }] },
      'POST /models/test': { modelId: 'm1', latencyMs: 12 },
    })
    const models = store()

    const candidates = await models.fetchRemote({ baseUrl: 'https://x/v1', apiKey: 'k' })
    expect(candidates.map((m) => m.id)).toEqual(['m1', 'm2'])

    const result = await models.test({ baseUrl: 'https://x/v1', secretRef: 'sec_1', modelId: 'm1' })
    expect(result?.modelId).toBe('m1')
    expect(calls.some((c) => c.method === 'POST' && c.path === '/models/test')).toBe(true)
  })

  it('reports a failed test without throwing', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ status: 502, json: async () => ({ error: 'upstream_unreachable' }) }) as Response),
    )
    const models = store()
    const result = await models.test({ baseUrl: 'https://x/v1', modelId: 'm1' })
    expect(result).toBeNull()
    expect(models.error).not.toBe('')
  })
})

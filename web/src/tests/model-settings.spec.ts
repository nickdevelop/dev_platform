// Tests for the model settings section: it renders services/models, fetches
// candidate models from a service, and runs a model test.
import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ModelSettings from '../features/settings/ModelSettings.vue'
import { useAuthStore } from '../stores/auth'

type Call = { method: string; path: string; body: any }

const services = {
  services: [
    {
      id: 1,
      name: 'Local',
      baseUrl: 'http://localhost:11434/v1',
      hasKey: false,
      secretRef: '',
      enabled: true,
      models: [
        {
          id: 10,
          serviceId: 1,
          modelId: 'llama3',
          name: 'Llama 3',
          supportsText: true,
          supportsImage: false,
          supportsTools: true,
          contextTokens: 8192,
          outputTokens: 2048,
          enabled: true,
          usable: true,
        },
      ],
    },
  ],
}

function stubFetch(extra: Record<string, unknown> = {}) {
  const calls: Call[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input).replace('/api', '')
      const method = init?.method ?? 'GET'
      const body = init?.body ? JSON.parse(String(init.body)) : undefined
      calls.push({ method, path, body })
      const key = `${method} ${path}`
      if (key in extra) return { status: 200, json: async () => extra[key] } as Response
      if (key === 'GET /model-services') return { status: 200, json: async () => services } as Response
      return { status: 200, json: async () => ({}) } as Response
    }),
  )
  return calls
}

async function mountView() {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().$patch({ authenticated: true, csrfToken: 'csrf-1' })
  const wrapper = mount(ModelSettings, { global: { plugins: [pinia] } })
  await flushPromises()
  return wrapper
}

afterEach(() => vi.unstubAllGlobals())

describe('ModelSettings', () => {
  it('renders services and their models', async () => {
    stubFetch()
    const wrapper = await mountView()
    expect(wrapper.text()).toContain('Local')
    expect(wrapper.text()).toContain('Llama 3')
  })

  it('fetches candidate models and adds a selected one', async () => {
    const calls = stubFetch({ 'POST /model-services/fetch-models': { models: [{ id: 'new-model' }] } })
    const wrapper = await mountView()

    await wrapper.find('[data-fetch-models="1"]').trigger('click')
    await flushPromises()
    const candidate = wrapper.find('[data-candidate="new-model"] input')
    expect(candidate.exists()).toBe(true)
    await candidate.setValue(true)

    await wrapper.find('[data-add-selected="1"]').trigger('click')
    await flushPromises()
    const add = calls.find((c) => c.method === 'POST' && c.path === '/model-services/1/models')
    expect(add?.body).toMatchObject({ modelId: 'new-model', enabled: true })
  })

  it('tests a model with the saved service config', async () => {
    const calls = stubFetch({ 'POST /models/test': { modelId: 'llama3', latencyMs: 5 } })
    const wrapper = await mountView()

    await wrapper.find('[data-test-model="10"]').trigger('click')
    await flushPromises()
    expect(calls.some((c) => c.method === 'POST' && c.path === '/models/test')).toBe(true)
    expect(wrapper.text()).toContain('正常')
  })
})

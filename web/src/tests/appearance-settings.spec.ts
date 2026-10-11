// Tests for the appearance settings section: it renders every preset, saves a
// change through the API and applies it to the document, and keeps the last
// good value when the save fails.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AppearanceSettings from '../components/settings/AppearanceSettings.vue'
import { useAuthStore } from '../stores/auth'
import { useSettingsStore } from '../stores/settings'

const defaultAppearance = {
  theme: 'light',
  font: 'system',
  customFont: '',
  fontSize: 14,
  density: 'standard',
}

function stubFetch(putStatus = 200) {
  const calls: { method: string; path: string; body: unknown }[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      const method = init?.method ?? 'GET'
      const body = init?.body ? JSON.parse(String(init.body)) : undefined
      calls.push({ method, path: url.replace('/api', ''), body })
      if (putStatus !== 200 && method === 'PUT') {
        return { status: putStatus, json: async () => ({ error: 'server_error' }) } as Response
      }
      return {
        status: 200,
        json: async () => ({ ...defaultAppearance, ...(body ?? {}) }),
      } as Response
    }),
  )
  return calls
}

async function mountSettings() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.$patch({ authenticated: true, csrfToken: 'csrf-1' })
  const wrapper = mount(AppearanceSettings, { global: { plugins: [pinia] } })
  await flushPromises()
  return { wrapper }
}

beforeEach(() => {
  localStorage.clear()
  document.documentElement.removeAttribute('data-theme')
  document.documentElement.removeAttribute('data-density')
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('AppearanceSettings', () => {
  it('renders every theme, font and density option', async () => {
    stubFetch()
    const { wrapper } = await mountSettings()

    expect(wrapper.findAll('[data-theme-option]')).toHaveLength(4)
    expect(wrapper.findAll('[data-density-option]')).toHaveLength(3)
    const fontOptions = wrapper.find('[data-font-select]').findAll('option')
    expect(fontOptions.map((o) => o.attributes('value'))).toEqual([
      'system',
      'sans',
      'serif',
      'mono',
      'custom',
    ])
    expect(wrapper.find('[data-font-size]').exists()).toBe(true)
  })

  it('saves a theme change and applies it to the document', async () => {
    const calls = stubFetch()
    const { wrapper } = await mountSettings()

    await wrapper.find('[data-theme-option="dark"]').trigger('click')
    await flushPromises()

    const put = calls.find((call) => call.method === 'PUT')
    expect(put?.body).toMatchObject({ theme: 'dark' })
    expect(document.documentElement.dataset.theme).toBe('dark')
    expect(useSettingsStore().appearance.theme).toBe('dark')
  })

  it('reveals the custom font field only for the custom font', async () => {
    stubFetch()
    const { wrapper } = await mountSettings()

    expect(wrapper.find('[data-custom-font]').exists()).toBe(false)
    await wrapper.find('[data-font-select]').setValue('custom')
    await flushPromises()
    expect(wrapper.find('[data-custom-font]').exists()).toBe(true)
  })

  it('keeps the previous appearance and reports an error when saving fails', async () => {
    stubFetch(500)
    const { wrapper } = await mountSettings()

    await wrapper.find('[data-theme-option="dark"]').trigger('click')
    await flushPromises()

    expect(useSettingsStore().appearance.theme).toBe('light')
    expect(wrapper.text()).toContain('服务内部错误')
  })
})

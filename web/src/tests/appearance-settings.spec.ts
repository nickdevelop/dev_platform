// Tests for the appearance settings section: it renders every preset, saves a
// change through the API and applies it to the document, and keeps the last
// good value when the save fails. The font face is built in, so the section
// offers no control for it.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AppearanceSettings from '../components/settings/AppearanceSettings.vue'
import { useAuthStore } from '../stores/auth'
import { useSettingsStore } from '../stores/settings'

const defaultAppearance = {
  theme: 'light',
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
  it('renders every theme and density option and no font control', async () => {
    stubFetch()
    const { wrapper } = await mountSettings()

    expect(wrapper.findAll('[data-theme-option]')).toHaveLength(4)
    expect(wrapper.findAll('[data-density-option]')).toHaveLength(3)
    expect(wrapper.findAll('legend').map((legend) => legend.text())).toEqual([
      '主题配色',
      '字号',
      '界面密度',
    ])
    expect(wrapper.find('[data-font-size]').exists()).toBe(true)
    expect(wrapper.find('[data-font-select]').exists()).toBe(false)
    expect(wrapper.find('[data-custom-font]').exists()).toBe(false)
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

  it('saves the font size without sending any font face', async () => {
    const calls = stubFetch()
    const { wrapper } = await mountSettings()

    await wrapper.find('[data-font-size]').setValue('18')
    await flushPromises()

    const put = calls.find((call) => call.method === 'PUT')
    expect(Object.keys(put?.body as object).sort()).toEqual(['density', 'fontSize', 'theme'])
    expect(put?.body).toMatchObject({ fontSize: 18 })
    expect(document.documentElement.style.getPropertyValue('--font-size-base')).toBe('18px')
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

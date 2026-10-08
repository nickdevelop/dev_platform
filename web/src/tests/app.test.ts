import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import App from '../App.vue'
import { createAppRouter } from '../router'

describe('App', () => {
  it('renders the routed placeholder view', async () => {
    const router = createAppRouter()
    await router.push('/')
    await router.isReady()

    const wrapper = mount(App, { global: { plugins: [router] } })

    expect(wrapper.find('main.home').exists()).toBe(true)
    expect(wrapper.text()).toContain('AI 开发项目管理平台')
  })
})

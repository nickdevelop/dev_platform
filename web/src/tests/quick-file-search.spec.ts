import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import QuickFileSearchDialog from '../features/chat/pane/QuickFileSearchDialog.vue'
import * as projectsApi from '../api/projects'

describe('QuickFileSearchDialog', () => {
  it('does not render when open is false', () => {
    const wrapper = mount(QuickFileSearchDialog, {
      props: {
        open: false,
        projectId: 1,
      },
    })
    expect(wrapper.find('[data-test="quick-file-search-dialog"]').exists()).toBe(false)
  })

  it('renders and fetches files when open is true', async () => {
    vi.spyOn(projectsApi, 'searchProjectFiles').mockResolvedValueOnce({
      files: [
        { path: 'src/main.ts', isDir: false },
        { path: 'src/app.vue', isDir: false },
        { path: 'src/components', isDir: true },
      ],
    })

    const wrapper = mount(QuickFileSearchDialog, {
      props: {
        open: true,
        projectId: 1,
      },
    })

    await flushPromises()

    expect(wrapper.find('[data-test="quick-file-search-dialog"]').exists()).toBe(true)
    const items = wrapper.findAll('[data-test="quick-search-item"]')
    // Only non-directory files should be listed
    expect(items.length).toBe(2)
    expect(items[0].text()).toContain('src/main.ts')
    expect(items[1].text()).toContain('src/app.vue')
  })

  it('navigates with ArrowDown/ArrowUp and selects with Enter', async () => {
    vi.spyOn(projectsApi, 'searchProjectFiles').mockResolvedValueOnce({
      files: [
        { path: 'src/first.ts', isDir: false },
        { path: 'src/second.ts', isDir: false },
      ],
    })

    const wrapper = mount(QuickFileSearchDialog, {
      props: {
        open: true,
        projectId: 1,
      },
    })

    await flushPromises()

    // Initially active index is 0
    let items = wrapper.findAll('[data-test="quick-search-item"]')
    expect(items[0].classes()).toContain('is-active')

    // Press ArrowDown
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown' }))
    await wrapper.vm.$nextTick()

    items = wrapper.findAll('[data-test="quick-search-item"]')
    expect(items[1].classes()).toContain('is-active')

    // Press Enter to select
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }))
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('select')?.[0]).toEqual(['src/second.ts'])
  })

  it('emits close on Escape key or backdrop click', async () => {
    vi.spyOn(projectsApi, 'searchProjectFiles').mockResolvedValueOnce({
      files: [],
    })

    const wrapper = mount(QuickFileSearchDialog, {
      props: {
        open: true,
        projectId: 1,
      },
    })

    await flushPromises()

    // Press Escape
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('close')).toBeTruthy()

    // Click backdrop
    await wrapper.find('.quick-search-backdrop').trigger('click')
    expect(wrapper.emitted('close')?.length).toBe(2)
  })
})

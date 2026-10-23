// Unit tests for the model picker in the composer. It is a compact trigger that
// opens a searchable list (not an always-visible dropdown): it reads the
// platform's persisted model snapshot for the backend, defaults to the first,
// reports its load status so the parent can gate sending, and renders nothing
// for a backend without model selection. The last list is cached per backend so
// a newly opened session shows its models immediately while the snapshot loads.
// The picker never refreshes the catalog: only Agent settings can do that.
import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ModelSelect from '../features/chat/composer/ModelSelect.vue'
import { clearModelCache } from '../shared/modelCache'
import type { BackendId } from '../api/agents'

const MODELS_URL = '/api/agent-backends/pi/models'

afterEach(() => {
  vi.unstubAllGlobals()
  clearModelCache()
})

function stubModels(payload: unknown, status = 200): string[] {
  const calls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL) => {
      calls.push(String(input))
      return { status, json: async () => payload } as Response
    }),
  )
  return calls
}

function mountSelect(
  overrides: Partial<{ modelSelection: boolean; modelValue: string; backend: BackendId }> = {},
) {
  return mount(ModelSelect, {
    props: {
      backend: 'pi',
      modelSelection: true,
      modelValue: '',
      ...overrides,
    },
  })
}

describe('ModelSelect', () => {
  it('shows a compact trigger for the loaded models and defaults to the first', async () => {
    const calls = stubModels({
      models: [
        { id: 'pi/a', name: 'Model A', provider: 'pi' },
        { id: 'pi/b', name: 'Model B', provider: 'pi' },
      ],
      updatedAt: '2026-10-13T12:00:00Z',
    })

    const wrapper = mountSelect()
    await flushPromises()

    expect(calls[0]).toBe(MODELS_URL)
    // Not a native dropdown, and the list stays closed until asked for.
    expect(wrapper.find('select').exists()).toBe(false)
    expect(wrapper.find('.model-menu').exists()).toBe(false)
    expect(wrapper.find('.model-trigger').text()).toContain('Model A')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['pi/a'])
    expect(wrapper.emitted('status')?.[0]).toEqual([true])

    await wrapper.find('.model-trigger').trigger('click')
    const options = wrapper.findAll('.model-option')
    expect(options).toHaveLength(2)
    expect(options[0].attributes('data-model-id')).toBe('pi/a')
    expect(options[0].attributes('aria-selected')).toBe('true')
  })

  it('picks a model from the open list and closes it', async () => {
    stubModels({
      models: [
        { id: 'pi/a', name: 'Model A', provider: 'pi' },
        { id: 'pi/b', name: 'Model B', provider: 'pi' },
      ],
    })
    const wrapper = mountSelect({ modelValue: 'pi/a' })
    await flushPromises()

    await wrapper.find('.model-trigger').trigger('click')
    await wrapper.findAll('.model-option')[1].trigger('click')

    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['pi/b'])
    expect(wrapper.find('.model-menu').exists()).toBe(false)
  })

  it('filters the list through the search field', async () => {
    stubModels({
      models: [
        { id: 'pi/a', name: 'Model A', provider: 'pi' },
        { id: 'pi/b', name: 'Model B', provider: 'pi' },
        { id: 'pi/c', name: 'Model C', provider: 'pi' },
        { id: 'pi/d', name: 'Model D', provider: 'pi' },
        { id: 'pi/e', name: 'Model E', provider: 'pi' },
        { id: 'pi/f', name: 'Model F', provider: 'pi' },
        { id: 'pi/g', name: 'Model G', provider: 'pi' },
      ],
    })
    const wrapper = mountSelect()
    await flushPromises()

    await wrapper.find('.model-trigger').trigger('click')
    await wrapper.find('.model-search').setValue('Model B')

    const options = wrapper.findAll('.model-option')
    expect(options).toHaveLength(1)
    expect(options[0].text()).toContain('Model B')
  })

  it('shows the failure message and reports a failed status', async () => {
    stubModels({}, 500)

    const wrapper = mountSelect()
    await flushPromises()

    expect(wrapper.text()).toContain('模型列表加载失败')
    expect(wrapper.emitted('status')?.[0]).toEqual([false])
  })

  it('renders nothing and loads nothing when model selection is unsupported', async () => {
    const calls = stubModels({ models: [] })

    const wrapper = mountSelect({ modelSelection: false })
    await flushPromises()

    expect(wrapper.find('.model-select').exists()).toBe(false)
    expect(calls).toHaveLength(0)
  })

  it('re-selects the first model when the parent clears the selection', async () => {
    stubModels({
      models: [
        { id: 'pi/a', name: 'Model A', provider: 'pi' },
        { id: 'pi/b', name: 'Model B', provider: 'pi' },
      ],
    })

    const wrapper = mountSelect({ modelValue: 'pi/b' })
    await flushPromises()
    // The parent already chose a model, so nothing is emitted on load.
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(wrapper.find('.model-trigger').text()).toContain('Model B')

    // Switching to a session that never chose a model clears the parent's
    // selection; it must default to the first again.
    await wrapper.setProps({ modelValue: '' })
    await flushPromises()
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['pi/a'])
  })

  it('shows the backend cache immediately, then refreshes it', async () => {
    stubModels({ models: [{ id: 'pi/a', name: 'Model A', provider: 'pi' }] })
    const first = mountSelect()
    await flushPromises()
    first.unmount()

    // The refresh hangs, so whatever is shown next can only be the cache.
    let resolveRefresh: (payload: unknown) => void = () => {}
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise<Response>((resolve) => {
            resolveRefresh = (payload) => resolve({ status: 200, json: async () => payload } as Response)
          }),
      ),
    )

    const second = mountSelect()
    await flushPromises()

    // The cached list is shown at once: no blank picker and sending is allowed.
    expect(second.find('.model-trigger').text()).toContain('Model A')
    expect(second.emitted('status')?.[0]).toEqual([true])

    resolveRefresh({
      models: [{ id: 'pi/b', name: 'Model B', provider: 'pi' }],
      updatedAt: '2026-10-13T12:00:00Z',
    })
    await flushPromises()
    expect(second.find('.model-trigger').text()).toContain('Model B')
  })

  it('keeps the cache and stays quiet when the refresh fails', async () => {
    stubModels({ models: [{ id: 'pi/a', name: 'Model A', provider: 'pi' }] })
    const first = mountSelect()
    await flushPromises()
    first.unmount()

    stubModels({}, 500)
    const second = mountSelect()
    await flushPromises()

    expect(second.find('.model-trigger').text()).toContain('Model A')
    expect(second.text()).not.toContain('模型列表加载失败')
    expect(second.emitted('status')?.at(-1)).toEqual([true])
  })

  it('never polls the model snapshot on its own', async () => {
    vi.useFakeTimers()
    const calls = stubModels({ models: [{ id: 'pi/a', name: 'Model A', provider: 'pi' }] })
    try {
      const wrapper = mountSelect()
      await vi.advanceTimersByTimeAsync(0)
      await flushPromises()

      expect(calls).toEqual([MODELS_URL])
      await vi.advanceTimersByTimeAsync(10000)
      await flushPromises()
      expect(calls).toEqual([MODELS_URL])
      wrapper.unmount()
    } finally {
      vi.useRealTimers()
    }
  })

  it('shows an empty state when the backend has never been synced', async () => {
    stubModels({ models: [], updatedAt: null })
    const wrapper = mountSelect()
    await flushPromises()

    expect(wrapper.find('.model-trigger').text()).toContain('暂无模型')
    await wrapper.find('.model-trigger').trigger('click')
    expect(wrapper.find('.model-menu').text()).toContain('尚未同步模型')
  })

  it('supports model variants and renders variant badges', async () => {
    stubModels({
      models: [
        {
          id: 'pi/a',
          name: 'Model A',
          provider: 'pi',
          variants: ['thinking', 'fast'],
        },
      ],
    })

    const wrapper = mountSelect({ modelValue: 'pi/a#thinking' })
    await flushPromises()

    // Trigger shows base name and variant badge
    expect(wrapper.find('.model-trigger-name').text()).toBe('Model A')
    const badge = wrapper.find('.model-variant-badge')
    expect(badge.exists()).toBe(true)
    expect(badge.text()).toBe('thinking')

    // Open menu to select another variant
    await wrapper.find('.model-trigger').trigger('click')
    const variantBtns = wrapper.findAll('.variant-btn')
    expect(variantBtns).toHaveLength(2)
    expect(variantBtns[0].classes()).toContain('selected')

    // Click 'fast' variant
    await variantBtns[1].trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['pi/a#fast'])
    expect(wrapper.emitted('picked')?.at(-1)).toEqual(['pi/a#fast'])
  })
})

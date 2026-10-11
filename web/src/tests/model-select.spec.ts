// Unit tests for the model picker in the composer: it loads the backend's
// models for the open instance, defaults to the first, reports its load status
// so the parent can gate sending, and renders nothing for a backend without
// model selection. The last list is cached per backend so a newly opened task
// shows its models immediately while a fresh list loads.
import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ModelSelect from '../components/ModelSelect.vue'
import { clearModelCache } from '../modelCache'
import type { BackendId } from '../api/agents'

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
  overrides: Partial<{ modelSelection: boolean; modelValue: string; instanceId: string; backend: BackendId }> = {},
) {
  return mount(ModelSelect, {
    props: {
      projectId: 1,
      instanceId: 'inst-1',
      backend: 'pi',
      modelSelection: true,
      modelValue: '',
      ...overrides,
    },
  })
}

describe('ModelSelect', () => {
  it('renders the loaded models, selects the first and emits it', async () => {
    const calls = stubModels({
      models: [
        { id: 'pi/a', name: 'Model A', provider: 'pi' },
        { id: 'pi/b', name: 'Model B', provider: 'pi' },
      ],
    })

    const wrapper = mountSelect()
    await flushPromises()

    expect(calls[0]).toBe('/api/projects/1/instances/inst-1/models')
    const options = wrapper.findAll('option')
    expect(options).toHaveLength(2)
    expect(options[0].attributes('value')).toBe('pi/a')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['pi/a'])
    expect(wrapper.emitted('status')?.[0]).toEqual([true])
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

    // Switching to a task on the same instance clears the parent's selection
    // without changing ModelSelect's props; it must still default again.
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

    const second = mountSelect({ instanceId: 'inst-2' })
    await flushPromises()

    // The cached list is shown at once: no blank picker and sending is allowed.
    expect(second.findAll('option').map((option) => option.attributes('value'))).toEqual(['pi/a'])
    expect(second.emitted('status')?.[0]).toEqual([true])

    resolveRefresh({ models: [{ id: 'pi/b', name: 'Model B', provider: 'pi' }] })
    await flushPromises()
    expect(second.findAll('option').map((option) => option.attributes('value'))).toEqual(['pi/b'])
  })

  it('keeps the cache and stays quiet when the refresh fails', async () => {
    stubModels({ models: [{ id: 'pi/a', name: 'Model A', provider: 'pi' }] })
    const first = mountSelect()
    await flushPromises()
    first.unmount()

    stubModels({}, 500)
    const second = mountSelect({ instanceId: 'inst-2' })
    await flushPromises()

    expect(second.findAll('option').map((option) => option.attributes('value'))).toEqual(['pi/a'])
    expect(second.text()).not.toContain('模型列表加载失败')
    expect(second.emitted('status')?.at(-1)).toEqual([true])
  })
})

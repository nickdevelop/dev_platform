// Unit tests for the project creation dialog. Editing and unregistering moved
// to the project settings and projects management pages.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ProjectManageDialog from '../features/projects/ProjectManageDialog.vue'
import { installTestCSRF } from './csrf-fixture'

function stubResponse(status: number, body: unknown = {}) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({ status, json: async () => body }) as Response),
  )
}

function mountDialog(kind: 'register' | 'create') {
  return mount(ProjectManageDialog, {
    props: { kind },
    attachTo: document.body,
  })
}

function confirmButton(wrapper: ReturnType<typeof mountDialog>) {
  return wrapper.find('.dialog-actions .confirm')
}

describe('ProjectManageDialog', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    installTestCSRF()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    document.body.innerHTML = ''
  })

  it('registers an existing directory', async () => {
    stubResponse(201, { id: 7, name: 'alpha', path: '/workspace/alpha', isGit: false })
    const wrapper = mountDialog('register')
    await flushPromises()

    await wrapper.find('#dialog-path').setValue('/workspace/alpha')
    await confirmButton(wrapper).trigger('click')
    await flushPromises()

    const fetchMock = globalThis.fetch as unknown as ReturnType<typeof vi.fn>
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/projects')
    expect(init.method).toBe('POST')
    expect(JSON.parse(String(init.body))).toEqual({ path: '/workspace/alpha' })
    expect(wrapper.emitted('submitted')).toEqual([['register']])
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })

  it('creates a project directory under a parent', async () => {
    stubResponse(201, { id: 8, name: 'demo', path: '/workspace/demo', isGit: false })
    const wrapper = mountDialog('create')
    await flushPromises()

    await wrapper.find('#dialog-project-name').setValue('demo')
    await wrapper.find('#dialog-path').setValue('/workspace')
    await confirmButton(wrapper).trigger('click')
    await flushPromises()

    const fetchMock = globalThis.fetch as unknown as ReturnType<typeof vi.fn>
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/projects/create')
    expect(init.method).toBe('POST')
    expect(JSON.parse(String(init.body))).toEqual({ parent: '/workspace', name: 'demo' })
    expect(wrapper.emitted('submitted')).toEqual([['create']])
    wrapper.unmount()
  })
})

// Unit tests for the project management dialog: edit prefills and saves the
// display name only, and unregister keeps the directory. Directory move and
// directory rename are deliberately not offered any more.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ProjectManageDialog from '../features/projects/ProjectManageDialog.vue'
import type { Project } from '../api/projects'
import { installTestCSRF } from './csrf-fixture'

const project: Project = { id: 7, name: 'alpha', path: '/workspace/alpha' } as Project

function stubResponse(status: number, body: unknown = {}) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({ status, json: async () => body }) as Response),
  )
}

function mountDialog(kind: 'edit' | 'unregister' | 'register' | 'create') {
  return mount(ProjectManageDialog, {
    props: { kind, project: kind === 'register' || kind === 'create' ? null : project },
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

  it('prefills the display name and saves it without touching the directory', async () => {
    stubResponse(200, { ...project, name: 'beta' })
    const wrapper = mountDialog('edit')
    await flushPromises()

    const nameInput = wrapper.find<HTMLInputElement>('#dialog-display-name')
    expect(nameInput.element.value).toBe('alpha')
    expect(wrapper.text()).toContain('/workspace/alpha')

    await nameInput.setValue('beta')
    await confirmButton(wrapper).trigger('click')
    await flushPromises()

    const fetchMock = globalThis.fetch as unknown as ReturnType<typeof vi.fn>
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/projects/7/name')
    expect(init.method).toBe('POST')
    expect(JSON.parse(String(init.body))).toEqual({ name: 'beta' })
    expect(wrapper.emitted('submitted')).toEqual([['edit']])
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })

  it('does not render directory move or rename controls', async () => {
    stubResponse(200, { ...project })
    const wrapper = mountDialog('edit')
    await flushPromises()

    expect(wrapper.find('#dialog-name').exists()).toBe(false)
    expect(wrapper.find('#dialog-confirm-name').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('重命名目录')
    expect(wrapper.text()).not.toContain('目标完整路径')
    wrapper.unmount()
  })

  it('needs no typed name to unregister and reports success', async () => {
    stubResponse(200, {})
    const wrapper = mountDialog('unregister')
    expect(wrapper.find('#dialog-confirm-name').exists()).toBe(false)
    expect(wrapper.text()).toContain('磁盘上的目录')
    expect(wrapper.text()).not.toContain('删除')

    await confirmButton(wrapper).trigger('click')
    await flushPromises()

    expect(wrapper.emitted('submitted')).toEqual([['unregister']])
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })
})

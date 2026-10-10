// Unit tests for the project management dialog: typed-name confirmation gates
// move, rename and delete; an unavailable OS trash requires an explicit
// permanent-delete checkbox; unregister keeps the directory and needs no name.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ProjectManageDialog from '../components/ProjectManageDialog.vue'
import type { Project } from '../api/projects'

const project: Project = { id: 7, name: 'alpha', path: '/workspace/alpha' } as Project

function stubResponse(status: number, body: unknown = {}) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({ status, json: async () => body }) as Response),
  )
}

function mountDialog(kind: 'move' | 'rename' | 'unregister' | 'delete' | 'register' | 'create') {
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
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    document.body.innerHTML = ''
  })

  it('keeps delete disabled until the typed project name matches', async () => {
    const wrapper = mountDialog('delete')
    expect(confirmButton(wrapper).attributes('disabled')).toBeDefined()

    await wrapper.find('#dialog-confirm-name').setValue('alph')
    expect(confirmButton(wrapper).attributes('disabled')).toBeDefined()

    await wrapper.find('#dialog-confirm-name').setValue('alpha')
    expect(confirmButton(wrapper).attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('requires the typed name for move and rename', async () => {
    const move = mountDialog('move')
    await move.find('#dialog-path').setValue('/workspace/moved')
    expect(confirmButton(move).attributes('disabled')).toBeDefined()
    await move.find('#dialog-confirm-name').setValue('alpha')
    expect(confirmButton(move).attributes('disabled')).toBeUndefined()
    move.unmount()

    const rename = mountDialog('rename')
    await rename.find('#dialog-name').setValue('beta')
    expect(confirmButton(rename).attributes('disabled')).toBeDefined()
    await rename.find('#dialog-confirm-name').setValue('alpha')
    expect(confirmButton(rename).attributes('disabled')).toBeUndefined()
    rename.unmount()
  })

  it('offers permanent delete only after the trash is reported unavailable', async () => {
    stubResponse(409, { error: 'trash_unavailable' })
    const wrapper = mountDialog('delete')
    await wrapper.find('#dialog-confirm-name').setValue('alpha')
    await confirmButton(wrapper).trigger('click')
    await flushPromises()

    expect(wrapper.find('.permanent-option input').exists()).toBe(true)
    expect(wrapper.text()).toContain('目录未被删除')
    expect(confirmButton(wrapper).attributes('disabled')).toBeDefined()

    await wrapper.find('.permanent-option input').setValue(true)
    expect(confirmButton(wrapper).attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('needs no typed name to unregister and reports success', async () => {
    stubResponse(200, {})
    const wrapper = mountDialog('unregister')
    expect(wrapper.find('#dialog-confirm-name').exists()).toBe(false)
    expect(wrapper.text()).toContain('磁盘上的目录')

    await confirmButton(wrapper).trigger('click')
    await flushPromises()

    expect(wrapper.emitted('submitted')).toEqual([['unregister']])
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })
})

// Unit tests for resource management: the API/store contract (including the
// write-only credential), the editor, the sync preview and the project
// settings integration. A stored credential value must never reach the DOM,
// the store state, or a second request.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ResourcesSection from '../components/ResourcesSection.vue'
import ResourceEditor from '../components/resources/ResourceEditor.vue'
import SyncPreview from '../components/resources/SyncPreview.vue'
import { useAuthStore } from '../stores/auth'
import { useResourcesStore } from '../stores/resources'
import type { ResourceDefinition, SyncPreview as SyncPreviewType } from '../api/resources'

function jsonResponse(status: number, payload: unknown): Response {
  return {
    status,
    json: async () => payload,
  } as unknown as Response
}

function resource(overrides: Partial<ResourceDefinition> = {}): ResourceDefinition {
  return {
    id: 1,
    kind: 'skill',
    name: 'commit-style',
    content: 'Use the imperative mood.',
    enabled: true,
    createdAt: '2026-10-11T10:00:00Z',
    updatedAt: '2026-10-11T10:00:00Z',
    ...overrides,
  }
}

describe('resources store', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    setActivePinia(createPinia())
    fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    useAuthStore().csrfToken = 'csrf-token'
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('loads the canonical resource list', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(200, { resources: [resource()] }))
    const store = useResourcesStore()

    await store.loadAll()

    expect(store.resources).toHaveLength(1)
    expect(store.byId(1)?.name).toBe('commit-style')
    expect(fetchMock.mock.calls[0][0]).toBe('/api/resources')
  })

  it('sends a write-only secret value once and never stores it', async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse(201, resource({ id: 2, kind: 'mcp', secretRef: 'sec_1' })))
      .mockResolvedValueOnce(jsonResponse(200, { resources: [resource({ id: 2, kind: 'mcp', secretRef: 'sec_1' })] }))
    const store = useResourcesStore()

    const created = await store.create({
      kind: 'mcp',
      name: 'server',
      content: '{}',
      enabled: true,
      secretValue: 'topsecret-value',
    })

    expect(created).toBe(true)
    const body = JSON.parse(fetchMock.mock.calls[0][1].body)
    expect(body.secretValue).toBe('topsecret-value')
    // Only the opaque reference is retained anywhere in state.
    expect(store.resources[0].secretRef).toBe('sec_1')
    expect(JSON.stringify(store.$state)).not.toContain('topsecret-value')
  })

  it('previews, applies and resolves synchronization', async () => {
    const preview: SyncPreviewType = {
      resourceId: 1,
      kind: 'skill',
      target: { projectId: 7, backendId: 'pi', path: '.pi/skills/commit-style.md' },
      state: 'conflict',
      changeSummary: 'write skill',
      conflict: {
        kind: 'external-drift',
        resourceId: 1,
        target: { projectId: 7, backendId: 'pi', path: '.pi/skills/commit-style.md' },
        detail: 'edited outside the platform',
      },
    }
    fetchMock
      .mockResolvedValueOnce(jsonResponse(200, { previews: [preview] }))
      .mockResolvedValueOnce(jsonResponse(200, { previews: [{ ...preview, state: 'applied', conflict: undefined }] }))
      .mockResolvedValueOnce(jsonResponse(200, { previews: [{ ...preview, state: 'applied', conflict: undefined }] }))
    const store = useResourcesStore()

    await store.preview(7)
    expect(store.previews).toHaveLength(1)
    expect(store.previews[0].state).toBe('conflict')

    await store.apply(7, true)
    expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toEqual({ force: true })

    await store.resolve(7, 1)
    expect(fetchMock.mock.calls[2][0]).toBe('/api/projects/7/resources/sync/resolve')
    expect(JSON.parse(fetchMock.mock.calls[2][1].body)).toEqual({ resourceId: 1 })
  })

  it('lists and toggles project associations', async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse(200, { resources: [resource()] }))
      .mockResolvedValueOnce(jsonResponse(204, null))
      .mockResolvedValueOnce(jsonResponse(200, { resources: [] }))
    const store = useResourcesStore()

    await store.loadProject(7)
    expect(store.isAssociated(1)).toBe(true)

    await store.disassociate(7, 1)
    expect(fetchMock.mock.calls[1][0]).toBe('/api/projects/7/resources/remove')
    expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toEqual({ resourceId: 1 })
    expect(store.isAssociated(1)).toBe(false)
  })
})

describe('ResourceEditor', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('never renders a stored credential value', () => {
    const wrapper = mount(ResourceEditor, {
      props: { resource: resource({ kind: 'mcp', secretRef: 'sec_1' }) },
    })

    expect(wrapper.find('[data-test="secret-stored"]').exists()).toBe(true)
    const input = wrapper.find('[data-test="secret-value"]').element as HTMLInputElement
    expect(input.value).toBe('')
    expect(wrapper.html()).not.toContain('topsecret')
  })

  it('submits a typed credential and omits an untouched one', async () => {
    const wrapper = mount(ResourceEditor, { props: { resource: null } })
    await wrapper.find('[data-test="resource-kind"]').setValue('mcp')
    await wrapper.find('[data-test="resource-name"]').setValue('server')
    await wrapper.find('[data-test="resource-content"]').setValue('{}')
    await wrapper.find('[data-test="secret-value"]').setValue('topsecret-value')
    await wrapper.find('form').trigger('submit')

    const body = wrapper.emitted('submit')?.[0]?.[0] as { secretValue?: string }
    expect(body.secretValue).toBe('topsecret-value')

    // Clearing the field leaves an existing credential untouched.
    const editing = mount(ResourceEditor, {
      props: { resource: resource({ kind: 'mcp', secretRef: 'sec_1' }) },
    })
    await editing.find('form').trigger('submit')
    const editingBody = editing.emitted('submit')?.[0]?.[0] as { secretValue?: string; secretRef?: string }
    expect(editingBody.secretValue).toBeUndefined()
    expect(editingBody.secretRef).toBe('sec_1')
  })
})

describe('SyncPreview', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('renders every state and emits resolve for a conflict', async () => {
    const previews: SyncPreviewType[] = [
      {
        resourceId: 1,
        kind: 'skill',
        target: { projectId: 7, backendId: 'pi', path: '.pi/skills/one.md' },
        state: 'pending',
        changeSummary: 'write skill',
      },
      {
        resourceId: 2,
        kind: 'prompt',
        target: { projectId: 7, path: '.devplatform/prompts/two.md' },
        state: 'conflict',
        changeSummary: 'write prompt',
        conflict: {
          kind: 'unmanaged-target',
          resourceId: 2,
          target: { projectId: 7, path: '.devplatform/prompts/two.md' },
          detail: 'not managed',
        },
      },
      {
        resourceId: 3,
        kind: 'mcp',
        target: { projectId: 7, backendId: 'pi', path: '.pi/mcp/three.json' },
        state: 'unsupported',
        changeSummary: 'backend pi does not support mcp',
      },
    ]
    const wrapper = mount(SyncPreview, { props: { previews } })

    expect(wrapper.text()).toContain('待首次应用')
    expect(wrapper.text()).toContain('冲突')
    expect(wrapper.text()).toContain('不支持')

    await wrapper.find('[data-test="resolve-2"]').trigger('click')
    expect(wrapper.emitted('resolve')?.[0]).toEqual([2])
  })

  it('emits apply with and without force', async () => {
    const wrapper = mount(SyncPreview, { props: { previews: [] } })

    await wrapper.find('[data-test="apply-sync"]').trigger('click')
    await wrapper.find('[data-test="force-sync"]').trigger('click')

    expect(wrapper.emitted('apply')).toEqual([[false], [true]])
  })
})

describe('ResourcesSection', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('lists resources and toggles the project association', async () => {
    const store = useResourcesStore()
    store.resources = [resource()]
    store.projectResources = []
    const loadProject = vi.spyOn(store, 'loadProject').mockResolvedValue(undefined as never)
    vi.spyOn(store, 'loadAll').mockResolvedValue(undefined as never)
    const associate = vi.spyOn(store, 'associate').mockResolvedValue(true as never)

    const wrapper = mount(ResourcesSection, { props: { projectId: 7 } })
    await flushPromises()

    expect(loadProject).toHaveBeenCalledWith(7)
    expect(wrapper.text()).toContain('commit-style')

    await wrapper.find('[data-test="associate-1"]').setValue(true)
    await flushPromises()
    expect(associate).toHaveBeenCalledWith(7, 1)
  })
})

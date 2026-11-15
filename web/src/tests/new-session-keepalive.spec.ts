// Regression tests for the kept-alive new-session form. The form stays mounted
// after the user leaves it, so it must neither carry the text of a session that
// was already created into the next visit, nor follow route changes made for
// another page (which would send the user to a new-session form instead).
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { Component } from 'vue'
import App from '../App.vue'
import { createAppRouter } from '../router'
import { useAgentsStore } from '../stores/agents'
import { useAuthStore } from '../stores/auth'
import { useProjectsStore } from '../stores/projects'
import { useSessionsStore } from '../stores/sessions'
import type { Capabilities } from '../api/sessions'

function capabilities(): Capabilities {
  return {
    streaming: true,
    sessionDiscovery: false,
    sessionContinue: false,
    stop: true,
    interrupt: false,
    modelSelection: false,
    toolCallEvents: false,
    toolConfirmation: false,
    sessionHistory: false,
    rawEvents: false,
  }
}

async function setup(path: string, stubs: Record<string, boolean | Component> = {}) {
  localStorage.clear()
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.$patch({ authenticated: true, sessionChecked: true })
  const projects = useProjectsStore()
  projects.projects = [
    { id: 1, name: 'alpha', path: '/workspace/alpha' } as never,
    { id: 2, name: 'beta', path: '/workspace/beta' } as never,
  ]
  vi.spyOn(projects, 'load').mockResolvedValue(undefined)

  const agents = useAgentsStore()
  agents.backends = [{ backend: 'pi', capabilities: capabilities(), label: 'Pi Agent', order: 2, serviceSupported: false }]
  agents.instances = [
    {
      id: 'inst-1',
      projectId: 1,
      backend: 'pi',
      directory: '/workspace/alpha',
      status: 'running',
      createdAt: '2026-10-09T00:00:00Z',
      updatedAt: '2026-10-09T00:00:00Z',
    },
  ]
  vi.spyOn(agents, 'loadBackends').mockResolvedValue(undefined)
  vi.spyOn(agents, 'loadInstances').mockResolvedValue(undefined)

  const router = createAppRouter()
  await router.push(path)
  await router.isReady()

  const wrapper = mount(App, {
    global: { plugins: [pinia, router], stubs: { ProjectTree: true, ...stubs } },
  })
  await flushPromises()
  return { wrapper, router, pinia }
}

describe('kept-alive new-session form', () => {
  beforeEach(() => {
    vi.unstubAllGlobals()
    vi.stubGlobal('fetch', vi.fn())
  })

  it('opens empty after a session was created from it', async () => {
    const { wrapper, router, pinia } = await setup('/projects/1/sessions/new', { SessionChatPanel: true })
    vi.spyOn(useSessionsStore(pinia), 'createSession').mockResolvedValue({ id: 'sess-new' } as never)

    await wrapper.find('textarea').setValue('第一条需求')
    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('session')

    await router.push({ name: 'session-new', params: { projectId: '1' } })
    await flushPromises()

    expect((wrapper.find('textarea').element as HTMLTextAreaElement).value).toBe('')
  })

  it('does not follow a route change made for another project\'s session', async () => {
    const { router } = await setup('/projects/1/sessions/new', { SessionChatPanel: true })

    await router.push({ name: 'session', params: { projectId: '2', sessionId: 'sess-b' } })
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('session')
    expect(router.currentRoute.value.params.projectId).toBe('2')
  })
})

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ProjectSettingsView from '../features/projects/ProjectSettingsView.vue'
import { useProjectsStore } from '../stores/projects'
import { installTestCSRF } from './csrf-fixture'
import type { ProjectWorktree } from '../api/worktrees'

const project = {
  id: 1,
  name: 'alpha',
  path: '/data/projects/alpha',
  isGit: true,
}

function worktree(overrides: Partial<ProjectWorktree> = {}): ProjectWorktree {
  return {
    id: 5,
    projectId: 1,
    path: '/data/projects/alpha-worktrees/feature-login',
    branch: 'feature/login',
    baseBranch: 'main',
    available: true,
    createdAt: '2026-11-10T10:00:00Z',
    updatedAt: '2026-11-10T10:00:00Z',
    ...overrides,
  }
}

function jsonResponse(body: unknown, status = 200): Response {
  return { status, json: async () => body } as Response
}

function mountView() {
  return mount(ProjectSettingsView, {
    props: { projectId: 1 },
    attachTo: document.body,
  })
}

describe('ProjectSettingsView', () => {
  let worktrees: ProjectWorktree[]

  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
    installTestCSRF()
    worktrees = []
    const projects = useProjectsStore()
    projects.projects = [{ ...project }]
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    document.body.innerHTML = ''
  })

  function stubApi() {
    const calls: Array<{ url: string; method: string; body?: unknown }> = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input)
        const method = (init?.method || 'GET').toUpperCase()
        const body = init?.body ? JSON.parse(String(init.body)) : undefined
        calls.push({ url, method, body })

        if (url.endsWith('/api/projects/1/git-info')) {
          return jsonResponse({ isGit: true, branch: 'main', dirty: false })
        }
        if (url.endsWith('/api/projects/1/git/branches')) {
          return jsonResponse({ branches: ['main', 'develop'], current: 'main' })
        }
        if (url.endsWith('/api/projects/1/worktrees') && method === 'GET') {
          return jsonResponse({ worktrees })
        }
        if (url.endsWith('/api/projects/1/worktrees') && method === 'POST') {
          const created = worktree({
            id: worktrees.length + 5,
            branch: String(body?.branch),
            baseBranch: String(body?.baseBranch || 'main'),
            path: String(body?.path || '/data/projects/alpha-worktrees/feature-login'),
          })
          worktrees = [...worktrees, created]
          return jsonResponse(created, 201)
        }
        if (url.includes('/api/projects/1/worktrees/') && method === 'DELETE') {
          const id = Number(url.split('/').pop())
          worktrees = worktrees.filter((entry) => entry.id !== id)
          return { status: 204, json: async () => ({}) } as Response
        }
        if (url.endsWith('/api/projects/1/name')) {
          return jsonResponse({ ...project, name: String(body?.name) })
        }
        return jsonResponse({}, 404)
      }),
    )
    return calls
  }

  it('shows the project identity and saves a display name', async () => {
    const calls = stubApi()
    const projects = useProjectsStore()
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('h1').text()).toBe('alpha')
    expect(wrapper.text()).toContain('/data/projects/alpha')
    expect(wrapper.find('[data-test="project-settings-git-status"]').text()).toContain('main')

    await wrapper.find('#project-display-name').setValue('alpha-renamed')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    const save = calls.find((call) => call.url.endsWith('/api/projects/1/name'))
    expect(save?.method).toBe('POST')
    expect(save?.body).toEqual({ name: 'alpha-renamed' })
    expect(projects.byId(1)?.name).toBe('alpha-renamed')
    expect(wrapper.text()).toContain('已保存')
  })

  it('creates a worktree and renders it in the list', async () => {
    const calls = stubApi()
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-test="worktree-empty"]').exists()).toBe(true)
    await wrapper.find('#worktree-branch').setValue('feature/login')
    await wrapper.find('[data-test="worktree-create-form"]').trigger('submit')
    await flushPromises()

    const create = calls.find((call) => call.url.endsWith('/api/projects/1/worktrees') && call.method === 'POST')
    expect(create?.body).toMatchObject({ branch: 'feature/login', baseBranch: 'main' })
    const rows = wrapper.findAll('[data-test="worktree-row"]')
    expect(rows).toHaveLength(1)
    expect(rows[0].text()).toContain('feature/login')
    expect(rows[0].text()).toContain('/data/projects/alpha-worktrees/feature-login')
  })

  it('removes a worktree after confirmation', async () => {
    worktrees = [worktree()]
    const calls = stubApi()
    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('[data-test="worktree-remove-btn"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('会被停止')
    await wrapper.find('[data-test="worktree-remove-confirm"]').trigger('click')
    await flushPromises()

    const remove = calls.find((call) => call.method === 'DELETE')
    expect(remove?.url).toBe('/api/projects/1/worktrees/5')
    expect(wrapper.findAll('[data-test="worktree-row"]')).toHaveLength(0)
  })

  it('explains a non-git project instead of offering a worktree form', async () => {
    const projects = useProjectsStore()
    projects.projects = [{ ...project, isGit: false }]
    stubApi()
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-test="worktree-not-git"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="worktree-create-form"]').exists()).toBe(false)
  })
})

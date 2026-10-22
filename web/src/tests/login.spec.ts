import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import App from '../App.vue'
import { createAppRouter } from '../router'
import { useAuthStore } from '../stores/auth'
import { installTestCSRF } from './csrf-fixture'

// The stub answers fetch calls in strict request order: each stubbed route is
// consumed exactly once, which lets a test script multi-step flows such as
// "rename fails with invalid_name, then succeeds with a valid name".
// Paths match exactly, so /api/projects never swallows /api/projects/1/... calls.
interface StubRoute {
  method: string
  path: string
  status: number
  body?: unknown
}

interface RecordedCall {
  method: string
  path: string
  body: unknown
  headers: Record<string, string>
}

function stubApi(routes: StubRoute[]) {
  const queue = [...routes]
  const calls: RecordedCall[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'http://localhost')
      const method = (init?.method ?? 'GET').toUpperCase()
      const headers: Record<string, string> = {}
      for (const [key, value] of Object.entries((init?.headers as Record<string, string>) ?? {})) {
        headers[key.toLowerCase()] = value
      }
      const body = init?.body ? JSON.parse(String(init.body)) : null
      calls.push({ method, path: url.pathname, body, headers })
      const index = queue.findIndex(
        (route) => route.method === method && url.pathname === route.path,
      )
      if (index === -1) {
        throw new Error(`unexpected ${method} ${url.pathname}`)
      }
      const route = queue.splice(index, 1)[0]
      return {
        status: route.status,
        json: async () => route.body,
      } as Response
    }),
  )
  return calls
}

function authenticatedSession() {
  return {
    method: 'GET',
    path: '/api/auth/session',
    status: 200,
    body: { authenticated: true, csrfToken: 'csrf-1', expiresAt: '2026-10-09T00:00:00Z' },
  } satisfies StubRoute
}

function projectsRoute(body: unknown) {
  return { method: 'GET', path: '/api/projects', status: 200, body } satisfies StubRoute
}

const demoProject = { id: 1, name: 'demo', path: '/workspace/demo' }
const secondProject = { id: 2, name: 'other', path: '/workspace/other' }

// Wrappers stay alive across a test otherwise, leaking DOM (and focus) into
// later assertions; every mounted wrapper is unmounted after each test.
const mountedWrappers: ReturnType<typeof mount>[] = []

async function mountApp(routes: StubRoute[], path = '/') {
  const calls = stubApi(routes)
  // The transport refuses to send a mutating request without a token source;
  // the app installs the real one in main.ts, which these specs never run. The
  // value matches the session stub below, as it does in production.
  installTestCSRF('csrf-1')
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createAppRouter()
  await router.push(path)
  await router.isReady()
  // Attach to the real document so focus management behaves like in a
  // browser; detached trees cannot receive focus.
  const wrapper = mount(App, { attachTo: document.body, global: { plugins: [pinia, router] } })
  mountedWrappers.push(wrapper)
  await flushPromises()
  return { wrapper, router, calls }
}

function dialog(wrapper: ReturnType<typeof mount>) {
  return wrapper.find('[role="dialog"]')
}

function buttonDisabled(element: Element) {
  return (element as HTMLButtonElement).disabled
}

afterEach(() => {
  for (const wrapper of mountedWrappers) wrapper.unmount()
  mountedWrappers.length = 0
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('login and session flows', () => {
  it('redirects unauthenticated users from the projects route to the login view', async () => {
    const { wrapper, router } = await mountApp([
      {
        method: 'GET',
        path: '/api/auth/session',
        status: 200,
        body: { authenticated: false, csrfToken: 'csrf-1' },
      },
    ])

    expect(router.currentRoute.value.name).toBe('login')
    const passwordInput = wrapper.find('input[type="password"]')
    expect(passwordInput.exists()).toBe(true)
    // The password field is labeled for screen readers.
    expect(wrapper.find('label[for="login-password"]').exists()).toBe(true)
  })

  it('logs in with the shared password and shows the projects list', async () => {
    const { wrapper, router, calls } = await mountApp([
      {
        method: 'GET',
        path: '/api/auth/session',
        status: 200,
        body: { authenticated: false, csrfToken: 'csrf-1' },
      },
      {
        method: 'POST',
        path: '/api/auth/login',
        status: 200,
        body: { authenticated: true, csrfToken: 'csrf-2', expiresAt: '2026-10-09T00:00:00Z' },
      },
      { method: 'GET', path: '/api/projects', status: 200, body: { projects: [demoProject] } },
    ])

    await wrapper.find('input[type="password"]').setValue('lan-password')
    await wrapper.find('form.login-form').trigger('submit')
    await flushPromises()

    expect(router.currentRoute.value.path).toBe('/')
    expect(wrapper.text()).toContain('demo')

    const login = calls.find((call) => call.method === 'POST' && call.path === '/api/auth/login')
    expect(login).toBeDefined()
    expect(login?.body).toEqual({ password: 'lan-password' })
    // Login carries the CSRF token from the session endpoint.
    expect(login?.headers['x-csrf-token']).toBe('csrf-1')
    expect(calls.some((call) => call.method === 'GET' && call.path === '/api/projects')).toBe(true)
  })

  it('returns the user to the login view when an API call reports session expiry', async () => {
    const { wrapper, router } = await mountApp([
      authenticatedSession(),
      { method: 'GET', path: '/api/projects', status: 401, body: { error: 'unauthorized' } },
    ])

    expect(router.currentRoute.value.name).toBe('login')
    expect(wrapper.find('input[type="password"]').exists()).toBe(true)
    const auth = useAuthStore()
    expect(auth.isAuthenticated).toBe(false)
  })

  it('shows the wrong-password error, not a session-expired message', async () => {
    const { wrapper, router } = await mountApp([
      {
        method: 'GET',
        path: '/api/auth/session',
        status: 200,
        body: { authenticated: false, csrfToken: 'csrf-1' },
      },
      {
        method: 'POST',
        path: '/api/auth/login',
        status: 401,
        body: { error: 'invalid_credentials' },
      },
    ])

    await wrapper.find('input[type="password"]').setValue('wrong-password')
    await wrapper.find('form.login-form').trigger('submit')
    await flushPromises()

    // The user stays on the login view and sees the credentials error, not
    // the session-expiry message (which a 401 would otherwise imply).
    expect(router.currentRoute.value.name).toBe('login')
    const alert = wrapper.find('.login-error[role="alert"]')
    expect(alert.exists()).toBe(true)
    expect(alert.text()).toContain('密码错误')
    expect(alert.text()).not.toContain('登录状态已失效')
  })

  it('logs out and returns to the login view', async () => {
    const { wrapper, router, calls } = await mountApp([
      authenticatedSession(),
      { method: 'GET', path: '/api/projects', status: 200, body: { projects: [demoProject] } },
      {
        method: 'POST',
        path: '/api/auth/logout',
        status: 200,
        body: { authenticated: false, csrfToken: 'csrf-2' },
      },
    ])

    await wrapper.find('button.logout').trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('login')
    const auth = useAuthStore()
    expect(auth.isAuthenticated).toBe(false)
    const logout = calls.find((call) => call.path === '/api/auth/logout')
    expect(logout?.headers['x-csrf-token']).toBe('csrf-1')
  })
})

describe('project management', () => {
  // Three list responses cover the initial load plus two reloads triggered
  // by successful mutations; tests append their own POST routes. Project
  // actions live in the project settings pane, so tests that use them mount
  // /projects/1 and answer the settings pane's agent loads.
  function mountWithProjects(extraRoutes: StubRoute[] = [], path = '/') {
    return mountApp(
      [
        authenticatedSession(),
        projectsRoute({ projects: [demoProject, secondProject] }),
        projectsRoute({ projects: [demoProject, secondProject] }),
        projectsRoute({ projects: [demoProject, secondProject] }),
        ...extraRoutes,
      ],
      path,
    )
  }

  function settingsRoutes(): StubRoute[] {
    return [
      { method: 'GET', path: '/api/agent-backends', status: 200, body: { backends: [] } },
      { method: 'GET', path: '/api/agent-backends/diagnostics', status: 200, body: { diagnostics: [] } },
      { method: 'GET', path: '/api/projects/1/agent-backends', status: 200, body: { backends: [] } },
      {
        method: 'GET',
        path: '/api/agent-services/opencode-v2',
        status: 200,
        body: {
          status: {
            backend: 'opencode-v2',
            supported: true,
            state: 'running',
            endpoint: 'http://127.0.0.1:4096',
            pid: 1,
            version: '2.0.24',
            executable: '/usr/bin/opencode',
            detail: '',
            checkedAt: '2026-10-13T00:00:00Z',
          },
        },
      },
    ]
  }

  it('filters the project list by name', async () => {
    const { wrapper } = await mountWithProjects()

    await wrapper.find('input[type="search"]').setValue('dem')
    expect(wrapper.text()).toContain('demo')
    expect(wrapper.text()).not.toContain('other')

    await wrapper.find('input[type="search"]').setValue('no-such-project')
    expect(wrapper.text()).toContain('没有匹配的项目')
  })

  it('unregisters a project without touching its directory and needs no typed name', async () => {
    const { wrapper, calls, router } = await mountWithProjects(
      [
        ...settingsRoutes(),
        { method: 'POST', path: '/api/projects/1/unregister', status: 204 },
      ],
      '/projects/1/settings',
    )

    // The only removal action is unregister; there is no directory deletion.
    expect(wrapper.find('button.delete').exists()).toBe(false)
    const unregisterButton = wrapper.find('button.unregister')
    expect(unregisterButton.text()).toBe('解除登记')

    await unregisterButton.trigger('click')
    const unregisterDialog = dialog(wrapper)
    expect(unregisterDialog.attributes('aria-label')).toContain('保留目录')
    expect(unregisterDialog.text()).toContain('会被完整保留')
    expect(unregisterDialog.find('label[for="dialog-confirm-name"]').exists()).toBe(false)
    const unregisterConfirm = unregisterDialog.find('button.confirm')
    expect(buttonDisabled(unregisterConfirm.element)).toBe(false)
    await unregisterConfirm.trigger('click')
    await flushPromises()
    expect(calls.some((call) => call.path === '/api/projects/1/unregister')).toBe(true)
    expect(dialog(wrapper).exists()).toBe(false)
    // The unregistered project's page is closed and the shell returns home.
    expect(router.currentRoute.value.name).toBe('home')
  })

  it('registers a project directory', async () => {
    const { wrapper, calls } = await mountWithProjects([
      { method: 'POST', path: '/api/projects', status: 201, body: { id: 9, name: 'demo', path: '/workspace/demo' } },
    ])

    await wrapper.find('button.register').trigger('click')
    await flushPromises()
    const registerDialog = dialog(wrapper)
    await registerDialog.find('#dialog-path').setValue('/workspace/demo')
    await registerDialog.find('button.confirm').trigger('click')
    await flushPromises()

    const register = calls.find((call) => call.method === 'POST' && call.path === '/api/projects')
    expect(register?.body).toEqual({ path: '/workspace/demo' })
    expect(register?.headers['x-csrf-token']).toBe('csrf-1')
  })

  it('moves focus into the dialog when it opens', async () => {
    const { wrapper } = await mountWithProjects(settingsRoutes(), '/projects/1/settings')

    await wrapper.find('button.unregister').trigger('click')
    await flushPromises()

    expect(document.activeElement).not.toBeNull()
    expect(dialog(wrapper).element.contains(document.activeElement)).toBe(true)
  })
})

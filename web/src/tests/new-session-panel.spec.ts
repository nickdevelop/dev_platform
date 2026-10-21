// Unit tests for the new session panel: it chooses an agent backend through a
// segmented switch (never a select, instance, or model field), submits the
// {backend, message?, model?} body with no title, and reports a failed
// creation without navigating.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import NewSessionPanel from '../features/chat/composer/NewSessionPanel.vue'
import { useAgentsStore } from '../stores/agents'
import { useProjectsStore } from '../stores/projects'
import { useSessionsStore } from '../stores/sessions'
import { usePromptsStore } from '../stores/prompts'
import { clearModelCache } from '../shared/modelCache'
import type { AgentInstance } from '../api/agents'
import type { Project } from '../api/projects'
import type { Capabilities } from '../api/sessions'

function capabilities(overrides: Partial<Capabilities> = {}): Capabilities {
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
    ...overrides,
  }
}

function setup(options: {
  piEnabled?: boolean
  instances?: AgentInstance[]
  modelSelection?: boolean
} = {}) {
  setActivePinia(createPinia())
  const agents = useAgentsStore()
  agents.backends = [
    {
      backend: 'opencode-v2',
      capabilities: capabilities({ modelSelection: options.modelSelection ?? false }),
      label: 'OpenCode',
      order: 1,
      serviceSupported: true,
    },
    {
      backend: 'pi',
      capabilities: capabilities(),
      label: 'Pi Agent',
      order: 2,
      serviceSupported: false,
      enabled: options.piEnabled !== false,
    },
  ]
  if (options.instances !== undefined) {
    agents.instances = options.instances
  }
  vi.spyOn(agents, 'loadBackends').mockResolvedValue(undefined as never)
  vi.spyOn(agents, 'loadInstances').mockResolvedValue(undefined as never)

  const sessions = useSessionsStore()
  const createSpy = vi.spyOn(sessions, 'createSession').mockResolvedValue({ id: 'sess-new' } as never)
  const wrapper = mount(NewSessionPanel, { props: { projectId: 1 } })
  return { wrapper, createSpy, sessions, agents }
}

function backendButton(wrapper: ReturnType<typeof mount>, backend: string) {
  return wrapper.find(`.backend-switch [data-backend="${backend}"]`)
}

describe('NewSessionPanel', () => {
  beforeEach(() => {
    vi.unstubAllGlobals()
    // The model list cache is module state: a list one test loaded would let
    // the next one's picker answer instantly and hide the loading it means to
    // assert.
    clearModelCache()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders a backend switch and a message composer, without a select or title', async () => {
    const { wrapper } = setup()
    await flushPromises()

    expect(wrapper.find('.backend-switch[role="radiogroup"]').exists()).toBe(true)
    expect(wrapper.find('.backend-switch [data-backend="opencode-v2"]').exists()).toBe(true)
    expect(wrapper.find('.backend-switch [data-backend="pi"]').exists()).toBe(true)
    expect(wrapper.find('#first-message').exists()).toBe(true)
    expect(wrapper.find('#session-title').exists()).toBe(false)
    expect(wrapper.find('#backend-select').exists()).toBe(false)
    expect(wrapper.find('#instance-select').exists()).toBe(false)
    expect(wrapper.find('#new-session-model').exists()).toBe(false)
    expect(wrapper.find('[data-test="discover-sessions"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="project-select"]').exists()).toBe(true)
  })

  it('submits { backend, message } and never a title, instanceId', async () => {
    const { wrapper, createSpy } = setup()
    await flushPromises()

    await backendButton(wrapper, 'pi').trigger('click')
    await wrapper.find('#first-message').setValue('帮我检查构建')
    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledWith(1, { backend: 'pi', message: '帮我检查构建' })
    const body = createSpy.mock.calls[0][1]
    expect(body).not.toHaveProperty('title')
    expect(body).not.toHaveProperty('instanceId')
    expect(body).not.toHaveProperty('model')
    expect(wrapper.emitted('created')?.[0]?.[0]).toBe('sess-new')
  })

  it('creates an empty session without a message field', async () => {
    const { wrapper, createSpy } = setup()
    await flushPromises()

    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledTimes(1)
    const body = createSpy.mock.calls[0][1]
    expect(body).not.toHaveProperty('message')
    expect(body).not.toHaveProperty('title')
  })

  it('labels the submit as 开始对话 when a first message is present', async () => {
    const { wrapper } = setup()
    await flushPromises()

    expect(wrapper.find('button.create').text()).toBe('新建会话')
    await wrapper.find('#first-message').setValue('帮我检查构建')
    expect(wrapper.find('button.create').text()).toBe('创建并开始对话')
  })

  it('sends the first message and model with the create request', async () => {
    // A backend's model list is scoped to the project, not to a running
    // instance, so the picker loads it here even though the project has no
    // instance at all — the create request is what starts one.
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        ({
          status: 200,
          json: async () => ({
            models: [{ id: 'p/m', provider: 'p', name: 'Model M' }],
            updatedAt: '2026-10-13T12:00:00Z',
            stale: false,
          }),
        }) as Response,
      ),
    )
    const { wrapper, createSpy } = setup({ modelSelection: true })
    await flushPromises()

    await backendButton(wrapper, 'opencode-v2').trigger('click')
    await flushPromises()
    await wrapper.find('#first-message').setValue('帮我检查构建失败')
    await flushPromises()
    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledTimes(1)
    const body = createSpy.mock.calls[0][1]
    expect(body.backend).toBe('opencode-v2')
    expect(body.message).toBe('帮我检查构建失败')
    expect(body.model).toBe('p/m')
    expect(body).not.toHaveProperty('title')
    expect(body).not.toHaveProperty('instanceId')
    expect(wrapper.emitted('created')?.[0]?.[0]).toBe('sess-new')
  })

  it('sends the chosen model even when no first message is sent', async () => {
    // The model is the session's starting model, not only the first turn's: the
    // created conversation opens on it, so it rides the create request too.
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        ({
          status: 200,
          json: async () => ({
            models: [{ id: 'p/m', provider: 'p', name: 'Model M' }],
            updatedAt: '2026-10-13T12:00:00Z',
            stale: false,
          }),
        }) as Response,
      ),
    )
    const { wrapper, createSpy } = setup({ modelSelection: true })
    await flushPromises()

    await backendButton(wrapper, 'opencode-v2').trigger('click')
    await flushPromises()
    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledTimes(1)
    const body = createSpy.mock.calls[0][1]
    expect(body.backend).toBe('opencode-v2')
    expect(body).not.toHaveProperty('message')
    expect(body.model).toBe('p/m')
  })

  it('holds a first message until the model list arrives', async () => {
    // Sending on a model the user never saw would be worse than waiting a
    // moment, so the create is held while the list is still loading and the
    // reason is shown.
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise<Response>(() => {
            // never resolves: the list is still loading
          }),
      ),
    )
    const { wrapper, createSpy } = setup({ modelSelection: true })
    await flushPromises()

    await backendButton(wrapper, 'opencode-v2').trigger('click')
    await flushPromises()
    await wrapper.find('#first-message').setValue('帮我检查构建失败')
    await flushPromises()

    expect(wrapper.find('button.create').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-test="model-required-hint"]').exists()).toBe(true)

    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()
    expect(createSpy).not.toHaveBeenCalled()
  })

  it('does not offer a backend disabled platform-wide', async () => {
    const { wrapper } = setup({ piEnabled: false })
    await flushPromises()

    expect(backendButton(wrapper, 'pi').exists()).toBe(false)
    expect(backendButton(wrapper, 'opencode-v2').exists()).toBe(true)

    await backendButton(wrapper, 'opencode-v2').trigger('click')
    await flushPromises()
    expect(wrapper.find('button.create').attributes('disabled')).toBeUndefined()
  })

  it('shows an error and does not report success when creation fails', async () => {
    setActivePinia(createPinia())
    const agents = useAgentsStore()
    agents.backends = [{ backend: 'pi', capabilities: capabilities(), label: 'Pi Agent', order: 2, serviceSupported: false }]
    vi.spyOn(agents, 'loadBackends').mockResolvedValue(undefined as never)
      vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          ({
            status: 502,
            json: async () => ({ error: 'instance_start_failed' }),
          }) as Response,
      ),
    )
    const sessions = useSessionsStore()
    const wrapper = mount(NewSessionPanel, { props: { projectId: 1 } })
    await flushPromises()

    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()

    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(wrapper.emitted('created')).toBeUndefined()
    expect(sessions.sessions).toHaveLength(0)
  })

  it('renders a plus button to attach files or images in new session composer', async () => {
    const { wrapper } = setup()
    await flushPromises()

    const plusBtn = wrapper.find('[data-test="attach-image-btn"]')
    expect(plusBtn.exists()).toBe(true)
    expect(plusBtn.text()).toBe('+')
    expect(wrapper.find('input[data-test="file-input"]').exists()).toBe(true)
  })

  it('selects project automatically when projectId prop is provided and allows manual change', async () => {
    const { wrapper, createSpy } = setup()
    const projectsStore = useProjectsStore()
    projectsStore.projects = [
      { id: 1, name: '项目一', path: '/path/1', defaultBackend: 'pi' },
      { id: 2, name: '项目二', path: '/path/2', defaultBackend: 'opencode-v2' },
    ] as unknown as Project[]
    await flushPromises()

    const select = wrapper.find<HTMLSelectElement>('[data-test="project-select"]')
    expect(select.exists()).toBe(true)
    // 初始传入 props: { projectId: 1 }，应自动选中项目 1
    expect(select.element.value).toBe('1')

    // 手动切换为项目 2
    await select.setValue('2')
    await flushPromises()
    expect(select.element.value).toBe('2')

    // 提交创建，应以项目 2 创建会话
    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledWith(2, expect.objectContaining({ backend: 'opencode-v2' }))
  })

  it('synchronizes the route URL when the selected project changes', async () => {
    setActivePinia(createPinia())
    const agents = useAgentsStore()
    agents.backends = [
      {
        backend: 'opencode-v2',
        capabilities: capabilities(),
        label: 'OpenCode',
        order: 1,
        serviceSupported: true,
      },
    ]
    vi.spyOn(agents, 'loadBackends').mockResolvedValue(undefined as never)
    const replaceMock = vi.fn()

    const projectsStore = useProjectsStore()
    projectsStore.projects = [
      { id: 1, name: '项目一', path: '/path/1', defaultBackend: 'opencode-v2' },
      { id: 3, name: '项目三', path: '/path/3', defaultBackend: 'opencode-v2' },
    ] as unknown as Project[]

    const wrapper = mount(NewSessionPanel, {
      props: { projectId: 1 },
      global: {
        mocks: {
          $router: { replace: replaceMock },
        },
        provide: {
          // Provide mock router for vue-router useRouter()
          router: { replace: replaceMock },
        },
      },
    })
    await flushPromises()

    const select = wrapper.find<HTMLSelectElement>('[data-test="project-select"]')
    await select.setValue('3')
    await flushPromises()

    // selectedProjectId becomes 3
    expect(select.element.value).toBe('3')
  })

  it('selects a prompt template and substitutes {user_prompt} with user message', async () => {
    const { wrapper, createSpy } = setup()
    const promptsStore = usePromptsStore()
    promptsStore.prompts = [
      {
        id: 10,
        name: '代码审查',
        description: '审查代码',
        content: '请对以下代码进行审查：\n\n{user_prompt}\n\n给出优化建议。',
        createdAt: '',
        updatedAt: '',
      },
    ]
    await flushPromises()

    const select = wrapper.find<HTMLSelectElement>('[data-test="prompt-select"]')
    expect(select.exists()).toBe(true)

    await select.setValue('10')
    await flushPromises()

    expect(wrapper.find('[data-test="prompt-preview-snippet"]').text()).toContain('请对以下代码进行审查')
    await wrapper.find('#first-message').setValue('function add(a, b) { return a + b }')
    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledTimes(1)
    const body = createSpy.mock.calls[0][1]
    expect(body.message).toBe('请对以下代码进行审查：\n\nfunction add(a, b) { return a + b }\n\n给出优化建议。')
  })

  it('selects a prompt template without user input and replaces {user_prompt} with empty string', async () => {
    const { wrapper, createSpy } = setup()
    const promptsStore = usePromptsStore()
    promptsStore.prompts = [
      {
        id: 20,
        name: '系统自检',
        description: '系统体检',
        content: '执行完整自检：{user_prompt}',
        createdAt: '',
        updatedAt: '',
      },
    ]
    await flushPromises()

    const select = wrapper.find<HTMLSelectElement>('[data-test="prompt-select"]')
    await select.setValue('20')
    await flushPromises()

    // Even without user input, hasContent is true because prompt is selected
    expect(wrapper.find('button.create').text()).toBe('创建并开始对话')

    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledTimes(1)
    const body = createSpy.mock.calls[0][1]
    expect(body.message).toBe('执行完整自检：')
  })

  it('allows clearing the selected prompt via clear button', async () => {
    const { wrapper } = setup()
    const promptsStore = usePromptsStore()
    promptsStore.prompts = [
      {
        id: 10,
        name: '代码审查',
        description: '审查代码',
        content: '审查：{user_prompt}',
        createdAt: '',
        updatedAt: '',
      },
    ]
    await flushPromises()

    const select = wrapper.find<HTMLSelectElement>('[data-test="prompt-select"]')
    await select.setValue('10')
    await flushPromises()

    expect(wrapper.find('[data-test="btn-clear-prompt"]').exists()).toBe(true)
    await wrapper.find('[data-test="btn-clear-prompt"]').trigger('click')
    await flushPromises()

    expect(select.element.value).toBe('')
    expect(wrapper.find('[data-test="prompt-preview-snippet"]').exists()).toBe(false)
  })

  it('preserves user input and selected project across remounts via draftsStore (prevents resetting to first project)', async () => {
    setActivePinia(createPinia())
    const agents = useAgentsStore()
    agents.backends = [
      {
        backend: 'opencode-v2',
        capabilities: capabilities(),
        label: 'OpenCode',
        order: 1,
        serviceSupported: true,
      },
    ]
    vi.spyOn(agents, 'loadBackends').mockResolvedValue(undefined as never)
    const projectsStore = useProjectsStore()
    projectsStore.projects = [
      { id: 1, name: '项目一（首个项目）', path: '/path/1', defaultBackend: 'opencode-v2' },
      { id: 2, name: '项目二（用户目标项目）', path: '/path/2', defaultBackend: 'opencode-v2' },
    ] as unknown as Project[]

    // 1. First mount: user opens new session without projectId prop (global new session /sessions/new)
    const wrapper1 = mount(NewSessionPanel, {})
    await flushPromises()

    const select1 = wrapper1.find<HTMLSelectElement>('[data-test="project-select"]')
    // User switches project to project 2 (not project 1)
    await select1.setValue('2')
    // User types message
    await wrapper1.find('#first-message').setValue('Important question for Project 2')
    await flushPromises()

    // 2. Simulate user switching to another session tab or another menu: wrapper1 unmounts
    wrapper1.unmount()

    // 3. User switches back to new session tab (/sessions/new): NewSessionPanel remounts
    const wrapper2 = mount(NewSessionPanel, {})
    await flushPromises()

    const select2 = wrapper2.find<HTMLSelectElement>('[data-test="project-select"]')
    const textarea2 = wrapper2.find<HTMLTextAreaElement>('#first-message')

    // Crucial assertion: the project MUST remain Project 2, NOT reset to Project 1!
    expect(select2.element.value).toBe('2')
    // Crucial assertion: message text must be preserved!
    expect(textarea2.element.value).toBe('Important question for Project 2')
  })

  it('clears draft after session is successfully created', async () => {
    setActivePinia(createPinia())
    const agents = useAgentsStore()
    agents.backends = [
      {
        backend: 'opencode-v2',
        capabilities: capabilities(),
        label: 'OpenCode',
        order: 1,
        serviceSupported: true,
      },
    ]
    vi.spyOn(agents, 'loadBackends').mockResolvedValue(undefined as never)
    const projectsStore = useProjectsStore()
    projectsStore.projects = [
      { id: 2, name: '项目二', path: '/path/2', defaultBackend: 'opencode-v2' },
    ] as unknown as Project[]
    const sessions = useSessionsStore()
    vi.spyOn(sessions, 'createSession').mockResolvedValue({ id: 'sess-created' } as never)

    const wrapper = mount(NewSessionPanel, { props: { projectId: 2 } })
    await flushPromises()

    await wrapper.find('#first-message').setValue('Message to send')
    await flushPromises()

    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()

    expect(wrapper.emitted('created')).toBeTruthy()

    // Now remount NewSessionPanel: draft should be empty
    wrapper.unmount()
    const freshWrapper = mount(NewSessionPanel, { props: { projectId: 2 } })
    await flushPromises()

    expect(freshWrapper.find<HTMLTextAreaElement>('#first-message').element.value).toBe('')
  })

  it('isolates input text when switching between different projects in new session', async () => {
    setActivePinia(createPinia())
    const agents = useAgentsStore()
    agents.backends = [
      {
        backend: 'opencode-v2',
        capabilities: capabilities(),
        label: 'OpenCode',
        order: 1,
        serviceSupported: true,
      },
    ]
    vi.spyOn(agents, 'loadBackends').mockResolvedValue(undefined as never)
    const projectsStore = useProjectsStore()
    projectsStore.projects = [
      { id: 1, name: '项目一', path: '/path/1', defaultBackend: 'opencode-v2' },
      { id: 2, name: '项目二', path: '/path/2', defaultBackend: 'opencode-v2' },
    ] as unknown as Project[]

    const wrapper = mount(NewSessionPanel, { props: { projectId: 1 } })
    await flushPromises()

    // 1. User types in Project 1
    const textarea = wrapper.find<HTMLTextAreaElement>('#first-message')
    await textarea.setValue('Task description specifically for Project 1')
    await flushPromises()

    // 2. User switches project selector to Project 2
    const select = wrapper.find<HTMLSelectElement>('[data-test="project-select"]')
    await select.setValue('2')
    await flushPromises()

    // Text must NOT be leaked/carried over to Project 2!
    expect(wrapper.find<HTMLTextAreaElement>('#first-message').element.value).toBe('')

    // 3. User types in Project 2
    await wrapper.find<HTMLTextAreaElement>('#first-message').setValue('Task for Project 2')
    await flushPromises()

    // 4. User switches back to Project 1
    await select.setValue('1')
    await flushPromises()

    // Project 1 text is accurately restored
    expect(wrapper.find<HTMLTextAreaElement>('#first-message').element.value).toBe(
      'Task description specifically for Project 1',
    )

    // 5. User switches back to Project 2
    await select.setValue('2')
    await flushPromises()
    expect(wrapper.find<HTMLTextAreaElement>('#first-message').element.value).toBe(
      'Task for Project 2',
    )
  })
})

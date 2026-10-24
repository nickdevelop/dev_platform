// Unit tests for ProjectSessionItem: two-row layout, compact agent logo with brand color,
// model tag, formatted time, delete action and keyboard accessibility.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ProjectSessionItem from '../features/projects/ProjectSessionItem.vue'
import { useAgentsStore } from '../stores/agents'
import { useSessionsStore } from '../stores/sessions'
import { backendFixtures } from './backend-fixtures'
import type { AgentSession } from '../api/sessions'

function mockSession(overrides: Partial<AgentSession> = {}): AgentSession {
  return {
    id: 'ses-1',
    projectId: 1,
    backend: 'opencode-v2',
    instanceId: 'inst-1',
    nativeSessionId: 'native-1',
    title: 'Test Session',
    status: 'active',
    turn: 'idle',
    capabilities: {
      streaming: false,
      sessionDiscovery: false,
      sessionContinue: false,
      stop: false,
      interrupt: false,
      modelSelection: false,
      toolCallEvents: false,
      toolConfirmation: false,
      sessionHistory: false,
      rawEvents: false,
    },
    createdAt: '2026-10-24T10:00:00Z',
    updatedAt: '2026-10-24T10:00:00Z',
    ...overrides,
  }
}

describe('ProjectSessionItem', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    const agentsStore = useAgentsStore()
    agentsStore.backends = [
      ...backendFixtures(),
      {
        backend: 'deepseek',
        capabilities: {
          streaming: true,
          sessionDiscovery: false,
          sessionContinue: false,
          stop: true,
          interrupt: false,
          modelSelection: true,
          toolCallEvents: true,
          toolConfirmation: false,
          sessionHistory: false,
          rawEvents: false,
        },
        enabled: true,
        label: 'DeepSeek',
        shortName: 'DS',
        order: 3,
        serviceSupported: false,
        color: '#1E40AF',
      },
    ]
  })

  it('renders Row 1 with simplified agent logo and session title', () => {
    const session = mockSession({ backend: 'opencode-v2', title: 'Feature Login' })
    const wrapper = mount(ProjectSessionItem, {
      props: { session },
    })

    const topRow = wrapper.find('.session-row-top')
    expect(topRow.exists()).toBe(true)

    // Logo shows "OC" for OpenCode with brand color style
    const logo = topRow.find('.agent-logo')
    expect(logo.text()).toBe('OC')
    expect(logo.attributes('style')).toContain('--logo-base: #10B981')

    // Title
    expect(topRow.find('.title').text()).toBe('Feature Login')
    expect(wrapper.find('[data-test="running-indicator"]').exists()).toBe(false)
  })

  it('renders DS logo for DeepSeek and PI logo for Pi Agent', () => {
    const dsWrapper = mount(ProjectSessionItem, {
      props: { session: mockSession({ backend: 'deepseek' }) },
    })
    expect(dsWrapper.find('.agent-logo').text()).toBe('DS')
    expect(dsWrapper.find('.agent-logo').attributes('style')).toContain('--logo-base: #1E40AF')

    const piWrapper = mount(ProjectSessionItem, {
      props: { session: mockSession({ backend: 'pi' }) },
    })
    expect(piWrapper.find('.agent-logo').text()).toBe('PI')
    expect(piWrapper.find('.agent-logo').attributes('style')).toContain('--logo-base: #F97316')
  })

  it('falls back to untitledLabel when session title is empty', () => {
    const wrapper = mount(ProjectSessionItem, {
      props: { session: mockSession({ title: '' }) },
    })
    expect(wrapper.find('.title').text()).toBe('未命名会话')
  })

  it('shows running indicator when session turn is running', () => {
    const wrapper = mount(ProjectSessionItem, {
      props: { session: mockSession({ turn: 'running' }) },
    })
    expect(wrapper.find('[data-test="running-indicator"]').exists()).toBe(true)
  })

  it('renders Row 2 with model tag, time and delete button', () => {
    const today = new Date()
    today.setHours(15, 45, 30, 0)
    const session = mockSession({
      model: 'anthropic/claude-3-5-sonnet',
      updatedAt: today.toISOString(),
    })
    const wrapper = mount(ProjectSessionItem, {
      props: { session },
    })

    const bottomRow = wrapper.find('.session-row-bottom')
    expect(bottomRow.exists()).toBe(true)

    // Model tag extracts model label
    const modelTag = bottomRow.find('[data-test="session-model"]')
    expect(modelTag.exists()).toBe(true)
    expect(modelTag.text()).toBe('claude-3-5-sonnet')
    expect(modelTag.attributes('title')).toBe('anthropic/claude-3-5-sonnet')

    // Time tag shows HH:mm:ss for today
    expect(bottomRow.find('.session-time').text()).toBe('15:45:30')

    // Delete button exists
    expect(bottomRow.find('[data-test="delete-session-btn"]').exists()).toBe(true)
  })

  it('formats past dates as YYYY-MM-DD on time tag', () => {
    const past = new Date(Date.now() - 3 * 24 * 3600 * 1000)
    const wrapper = mount(ProjectSessionItem, {
      props: { session: mockSession({ updatedAt: past.toISOString() }) },
    })
    expect(wrapper.find('.session-time').text()).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  })

  it('emits select when clicking the session or pressing Enter', async () => {
    const wrapper = mount(ProjectSessionItem, {
      props: { session: mockSession({ id: 'ses-42' }) },
    })

    await wrapper.find('.session-open').trigger('click')
    expect(wrapper.emitted('select')).toEqual([['ses-42']])

    await wrapper.find('.session-open').trigger('keydown.enter')
    expect(wrapper.emitted('select')).toEqual([['ses-42'], ['ses-42']])
  })

  it('emits delete when clicking delete button without bubbling select', async () => {
    const wrapper = mount(ProjectSessionItem, {
      props: { session: mockSession({ id: 'ses-42' }) },
    })

    const deleteBtn = wrapper.find('[data-test="delete-session-btn"]')
    await deleteBtn.trigger('click')

    expect(wrapper.emitted('delete')).toEqual([['ses-42']])
    expect(wrapper.emitted('select')).toBeUndefined()
  })

  it('applies selected styles when isSelected is true', () => {
    const wrapper = mount(ProjectSessionItem, {
      props: { session: mockSession(), isSelected: true },
    })
    expect(wrapper.find('.session-open').classes()).toContain('border-platform-primary')
  })
})

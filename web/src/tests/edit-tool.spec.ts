// Edit tool display: the store normalizes backend metadata into ChatEntry.diff
// and MessageView renders the desktop-style badge + diff body for both
// backends. opencode ships metadata.files; pi's is normalized server-side
// into the same shape, so the view never branches on backend.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import MessageView from '../components/MessageView.vue'
import { useSessionsStore } from '../stores/sessions'
import type { AgentSession, Capabilities } from '../api/sessions'
import type { ChatEntry } from '../stores/sessions'

function capabilities(): Capabilities {
  return {
    streaming: true,
    sessionDiscovery: false,
    sessionContinue: false,
    stop: true,
    interrupt: false,
    modelSelection: false,
    toolCallEvents: true,
    toolConfirmation: false,
    sessionHistory: false,
    rawEvents: false,
  }
}

function session(): AgentSession {
  return {
    id: 's1',
    projectId: 1,
    backend: 'pi',
    instanceId: 'inst-1',
    nativeSessionId: 'native-s1',
    title: 's1',
    status: 'active',
    turn: 'idle',
    capabilities: capabilities(),
    createdAt: '2026-10-09T00:00:00Z',
    updatedAt: '2026-10-09T00:00:00Z',
  }
}

function event(overrides: Record<string, unknown> = {}) {
  return {
    instanceId: 'inst-1',
    backend: 'pi',
    sequence: 1,
    timestamp: '2026-10-10T12:00:00Z',
    ...overrides,
  }
}

const PATCH = '--- a/foo.ts\n+++ b/foo.ts\n@@ -1,2 +1,2 @@\n context\n-removed\n+added\n'

describe('edit diff store normalization', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('keeps the first changed file from tool result metadata', () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session()]
    sessions.handleEvent(
      's1',
      JSON.stringify(
        event({
          type: 'tool_result',
          sequence: 1,
          toolResult: {
            callId: 'c1',
            output: 'Edited foo.ts',
            isError: false,
            metadata: { files: [{ file: 'foo.ts', patch: PATCH, additions: 1, deletions: 1, status: 'modified' }] },
          },
        }),
      ),
    )
    expect(sessions.messagesFor('s1')[0].diff).toEqual({ file: 'foo.ts', patch: PATCH, additions: 1, deletions: 1 })
  })

  it('drops zero-count and malformed diffs', () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session()]
    sessions.handleEvent(
      's1',
      JSON.stringify(
        event({
          type: 'tool_result',
          sequence: 1,
          toolResult: { callId: 'c1', output: 'ok', isError: false, metadata: { files: [{ file: 'a', additions: 0, deletions: 0 }] } },
        }),
      ),
    )
    sessions.handleEvent(
      's1',
      JSON.stringify(event({ type: 'tool_result', sequence: 2, toolResult: { callId: 'c2', output: 'ok', isError: false } })),
    )
    expect(sessions.messagesFor('s1')[0].diff).toBeUndefined()
    expect(sessions.messagesFor('s1')[1].diff).toBeUndefined()
  })
})

function editEntries(input: unknown, diff?: ChatEntry['diff'], output = 'Edited foo.ts'): ChatEntry[] {
  return [
    { id: 'call-1', kind: 'tool_call', role: 'assistant', text: 'edit', name: 'edit', input, streaming: false, callId: 'c1' },
    { id: 'res-1', kind: 'tool_result', role: 'tool', text: output, streaming: false, callId: 'c1', diff },
  ]
}

describe('MessageView edit tool block', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('renders badge and diff for opencode-style input', async () => {
    const wrapper = mount(MessageView, {
      props: {
        messages: editEntries(
          { path: 'foo.ts', oldString: 'a', newString: 'b' },
          { file: 'foo.ts', patch: PATCH, additions: 1, deletions: 1 },
        ),
        capabilities: capabilities(),
      },
    })
    await flushPromises()
    expect(wrapper.find('[data-test="edit-tool"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="diff-additions"]').text()).toBe('+1')
    expect(wrapper.find('[data-test="diff-deletions"]').text()).toBe('-1')
    expect(wrapper.find('[data-test="edit-path"]').text()).toBe('foo.ts')
    const diff = wrapper.find('[data-test="edit-diff"]')
    expect(diff.find('.diff-addition').text()).toContain('added')
    expect(diff.find('.diff-deletion').text()).toContain('removed')
  })

  it('resolves pi-style file_path input', async () => {
    const wrapper = mount(MessageView, {
      props: {
        messages: editEntries(
          { file_path: 'src/app.ts', edits: [{ oldText: 'a', newText: 'b' }] },
          { file: 'src/app.ts', patch: PATCH, additions: 3, deletions: 0 },
        ),
        capabilities: capabilities(),
      },
    })
    await flushPromises()
    expect(wrapper.find('[data-test="edit-path"]').text()).toBe('src/app.ts')
    expect(wrapper.find('[data-test="diff-additions"]').text()).toBe('+3')
    expect(wrapper.find('[data-test="diff-deletions"]').exists()).toBe(false)
  })

  it('falls back to plain output without diff metadata', async () => {
    const wrapper = mount(MessageView, {
      props: {
        messages: editEntries({ path: 'old.ts', oldString: 'a', newString: 'b' }, undefined, 'plain output'),
        capabilities: capabilities(),
      },
    })
    await flushPromises()
    expect(wrapper.find('[data-test="edit-tool"]').exists()).toBe(false)
    expect(wrapper.find('.tool-output').text()).toBe('plain output')
  })

  it('shows running state while the edit has no result', async () => {
    const entries: ChatEntry[] = [
      { id: 'call-1', kind: 'tool_call', role: 'assistant', text: 'edit', name: 'EDIT', input: { path: 'foo.ts' }, streaming: false, callId: 'c1' },
    ]
    const wrapper = mount(MessageView, { props: { messages: entries, capabilities: capabilities() } })
    await flushPromises()
    expect(wrapper.find('[data-test="edit-tool"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('运行中')
  })
})

// Coverage for the six workbench adjustments: tab bar, step groups + subagent
// popup, and the stepwise question queue.
import { describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import MessageView from '../components/MessageView.vue'
import QuestionQueue from '../components/QuestionQueue.vue'
import TaskTabs from '../components/TaskTabs.vue'
import { tabKey, useOpenTabsStore } from '../stores/openTabs'
import type { Capabilities } from '../api/sessions'
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

describe('step groups and subagent popup', () => {
  it('groups consecutive thinking and tool entries into one step card', () => {
    const entries: ChatEntry[] = [
      { id: 'u1', kind: 'message', role: 'user', text: 'hi', streaming: false },
      { id: 't1', kind: 'thinking', role: 'assistant', text: 'plan', streaming: false },
      {
        id: 'c1', kind: 'tool_call', role: 'assistant', text: 'read', name: 'read',
        callId: 'call-1', input: { path: '/a.md' }, streaming: false,
      },
      { id: 'r1', kind: 'tool_result', role: 'tool', text: 'ok', callId: 'call-1', streaming: false },
      { id: 'a1', kind: 'message', role: 'assistant', text: 'done', streaming: false },
    ]
    const wrapper = mount(MessageView, { props: { messages: entries, capabilities: capabilities() } })
    expect(wrapper.find('[data-test="step-group"]').exists()).toBe(true)
    // The tool block still renders inside the group.
    expect(wrapper.find('[data-test="tool-call"]').exists()).toBe(true)
    expect(wrapper.find('.step-title').text()).toContain('read')
    // The group starts folded: activity stays out of the way until the user
    // opens it by hand.
    expect(wrapper.find('[data-test="step-group"]').attributes('open')).toBeUndefined()
  })

  it('opens a dialog for subagent tool calls', async () => {
    const entries: ChatEntry[] = [
      {
        id: 'c1', kind: 'tool_call', role: 'assistant', text: 'task', name: 'task',
        callId: 'call-9', input: { description: 'sub job', prompt: 'do it' }, streaming: false,
      },
      {
        id: 'r1', kind: 'tool_result', role: 'tool',
        text: '<subagent sessionID="ses_child" state="completed">\n# report', callId: 'call-9', streaming: false,
      },
    ]
    const wrapper = mount(MessageView, { props: { messages: entries, capabilities: capabilities() } })
    expect(wrapper.find('[data-test="subagent-open"]').exists()).toBe(true)
    await wrapper.find('[data-test="subagent-open"]').trigger('click')
    const dialog = wrapper.find('[data-test="subagent-dialog"]')
    expect(dialog.exists()).toBe(true)
    expect(dialog.text()).toContain('ses_child')
    await dialog.find('[data-test="subagent-close"]').trigger('click')
    expect(wrapper.find('[data-test="subagent-dialog"]').exists()).toBe(false)
  })
})

describe('question queue', () => {
  it('answers one field at a time and submits once', async () => {
    const wrapper = mount(QuestionQueue, {
      props: {
        forms: [
          {
            id: 'frm_1', sessionId: 's1', title: 'pick',
            fields: [
              { key: 'a', type: 'string', title: 'first', required: true },
              { key: 'b', type: 'string', title: 'second', required: true },
            ],
          },
        ],
      },
    })
    // Only the first question shows with a next button, no submit yet.
    expect(wrapper.text()).toContain('first')
    expect(wrapper.text()).not.toContain('second')
    expect(wrapper.find('[data-test="submit-answer"]').exists()).toBe(false)
    await wrapper.find('.text-input').setValue('one')
    await wrapper.find('[data-test="question-next"]').trigger('click')
    expect(wrapper.text()).toContain('second')
    await wrapper.find('.text-input').setValue('two')
    await wrapper.find('[data-test="submit-answer"]').trigger('click')
    expect(wrapper.emitted('submit')).toEqual([[ 'frm_1', { a: 'one', b: 'two' } ]])
  })

  it('queues multiple forms one at a time', async () => {
    const wrapper = mount(QuestionQueue, {
      props: {
        forms: [
          { id: 'f1', sessionId: 's1', title: 'one', fields: [{ key: 'a', type: 'string', title: 'A' }] },
          { id: 'f2', sessionId: 's1', title: 'two', fields: [{ key: 'b', type: 'string', title: 'B' }] },
        ],
      },
    })
    expect(wrapper.find('[data-form-id="f1"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('表单 1/2')
    await wrapper.findAll('.queue-tab')[1].trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-form-id="f2"]').exists()).toBe(true)
  })
})

describe('task tabs and open-tabs store', () => {
  it('opens, labels and closes tabs', () => {
    localStorage.clear()
    setActivePinia(createPinia())
    const store = useOpenTabsStore()
    store.closeAllTabs()
    store.openTab(1, 's1', 'Task One')
    store.openTab(1, 's2', 'Task Two')
    expect(store.tabs).toHaveLength(2)
    store.updateTitle('s1', 'Renamed')
    expect(store.tabs[0]).toMatchObject({ kind: 'task', sessionId: 's1', title: 'Renamed' })
    store.closeTab('s1')
    expect(store.tabs.map(tabKey)).toEqual(['s2'])
  })

  it('renders tabs with active state and emits select/close', async () => {
    const wrapper = mount(TaskTabs, {
      props: {
        tabs: [
          { kind: 'task', projectId: 1, sessionId: 's1', title: 'One' },
          { kind: 'task', projectId: 1, sessionId: 's2', title: 'Two' },
        ],
        activeKey: 's2',
      },
    })
    const tabs = wrapper.findAll('[data-test="open-tab"]')
    expect(tabs).toHaveLength(2)
    expect(tabs[1].classes()).toContain('active')
    await tabs[0].find('.tab-select').trigger('click')
    expect(wrapper.emitted('select')?.[0]?.[0]).toMatchObject({ sessionId: 's1' })
    await tabs[0].find('[data-test="close-tab"]').trigger('click')
    expect(wrapper.emitted('close')?.[0]?.[0]).toMatchObject({ sessionId: 's1' })
  })
})

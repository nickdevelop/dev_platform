// Tool output folding: edit diffs and shell (and every other textual tool)
// outputs render the first 15 rows by default with a centered ▼ that expands to
// the full content and folds it back. See ToolOutput.vue, EditDiffBody.vue and
// ExpandIndicator.vue.
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ToolOutput from '../components/ToolOutput.vue'
import EditDiffBody from '../components/EditDiffBody.vue'
import MessageView from '../components/MessageView.vue'
import type { Capabilities } from '../api/sessions'
import type { ChatEntry } from '../stores/sessions'

function capabilities(overrides: Partial<Capabilities> = {}): Capabilities {
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
    ...overrides,
  }
}

function lines(count: number, prefix = 'line'): string {
  return Array.from({ length: count }, (_, index) => `${prefix}-${index + 1}`).join('\n')
}

describe('ToolOutput folding', () => {
  it('shows short output in full with no expand control', () => {
    const wrapper = mount(ToolOutput, { props: { text: lines(3) } })
    expect(wrapper.find('.tool-output').text()).toBe(lines(3))
    expect(wrapper.find('[data-test="output-more"]').exists()).toBe(false)
  })

  it('renders exactly 15 rows with a ▼ when the text is longer', () => {
    const text = lines(20)
    const wrapper = mount(ToolOutput, { props: { text } })
    const pre = wrapper.find('.tool-output')
    expect(pre.text()).toBe(lines(15))
    expect(pre.text()).not.toContain('line-16')

    const toggle = wrapper.find('[data-test="output-more-toggle"]')
    expect(toggle.exists()).toBe(true)
    expect(toggle.text()).toBe('▼')
    // The fade rides on the folded block, not on the expanded one.
    expect(wrapper.find('[data-test="output-more"]').classes()).toContain('is-collapsed')
  })

  it('expands to every row and folds back to 15', async () => {
    const text = lines(20)
    const wrapper = mount(ToolOutput, { props: { text } })
    await wrapper.find('[data-test="output-more-toggle"]').trigger('click')
    expect(wrapper.find('.tool-output').text()).toBe(text)
    expect(wrapper.find('[data-test="output-more-toggle"]').text()).toBe('▲')
    expect(wrapper.find('[data-test="output-more"]').classes()).not.toContain('is-collapsed')

    await wrapper.find('[data-test="output-more-toggle"]').trigger('click')
    expect(wrapper.find('.tool-output').text()).toBe(lines(15))
  })

  it('does not count a trailing newline as another row', () => {
    const wrapper = mount(ToolOutput, { props: { text: `${lines(15)}\n` } })
    expect(wrapper.find('[data-test="output-more"]').exists()).toBe(false)
  })

  it('marks an errored output', () => {
    const wrapper = mount(ToolOutput, { props: { text: 'boom', isError: true } })
    expect(wrapper.find('.tool-output').classes()).toContain('is-error')
  })
})

describe('EditDiffBody folding', () => {
  function patch(rowCount: number): string {
    const body = Array.from({ length: rowCount }, (_, index) => ` ctx-${index + 1}`)
    return `@@ -1,${rowCount} +1,${rowCount} @@\n${body.join('\n')}`
  }

  it('folds a long diff to 15 rows and expands to all', async () => {
    const wrapper = mount(EditDiffBody, { props: { patch: patch(20) } })
    expect(wrapper.findAll('.diff-row')).toHaveLength(15)
    expect(wrapper.text()).not.toContain('ctx-20')

    await wrapper.find('[data-test="output-more-toggle"]').trigger('click')
    expect(wrapper.findAll('.diff-row')).toHaveLength(21)
    expect(wrapper.text()).toContain('ctx-20')
  })

  it('leaves a short diff unfolded', () => {
    const wrapper = mount(EditDiffBody, { props: { patch: patch(4) } })
    expect(wrapper.find('[data-test="output-more"]').exists()).toBe(false)
  })
})

describe('MessageView tool output folding', () => {
  function shellEntries(output: string): ChatEntry[] {
    return [
      {
        id: 'c1', kind: 'tool_call', role: 'assistant', text: 'bash', name: 'bash',
        callId: 'c1', input: { command: 'ls' }, streaming: false,
      },
      { id: 'r1', kind: 'tool_result', role: 'tool', text: output, callId: 'c1', streaming: false },
    ]
  }

  it('folds a shell output in the activity pane and expands on demand', async () => {
    const wrapper = mount(MessageView, {
      props: { messages: shellEntries(lines(20, 'out')), capabilities: capabilities(), display: 'activity' },
    })
    const call = wrapper.find('[data-test="tool-call"]')
    expect(call.find('.tool-output').text()).not.toContain('out-16')

    await call.find('[data-test="output-more-toggle"]').trigger('click')
    expect(call.find('.tool-output').text()).toContain('out-20')
  })

  it('folds a shell output in the interleaved log once the card is open', async () => {
    const wrapper = mount(MessageView, {
      props: { messages: shellEntries(lines(20, 'out')), capabilities: capabilities() },
    })
    const call = wrapper.find('[data-test="tool-call"]')
    await call.find('summary').trigger('click')
    expect(call.find('.tool-output').text()).not.toContain('out-16')
    await call.find('[data-test="output-more-toggle"]').trigger('click')
    expect(call.find('.tool-output').text()).toContain('out-20')
  })
})

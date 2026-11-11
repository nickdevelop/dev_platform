// Tool call/result pairing: a call renders as a card that is answered by its
// result, and a call left without one must stop claiming to run once its turn
// is over. Legacy transcripts recorded a call twice, so the render model keeps
// one card per call id.
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import MessageView from '../features/chat/rendering/MessageView.vue'
import { buildLogModel, pendingToolLabel, stepSummary } from '../features/chat/rendering/logModel'
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

function call(id: string, callId: string, name = 'read'): ChatEntry {
  return { id, kind: 'tool_call', role: 'assistant', text: name, name, callId, input: { path: 'a.txt' }, streaming: false }
}

function result(id: string, callId: string, text: string): ChatEntry {
  return { id, kind: 'tool_result', role: 'tool', text, callId, streaming: false }
}

describe('tool call pairing', () => {
  it('keeps one card per call id when a legacy transcript repeats the call', () => {
    const model = buildLogModel([
      call('c1', 'call-1'),
      call('c2', 'call-2'),
      call('c1-again', 'call-1'),
      result('r1', 'call-1', 'AAA'),
      call('c2-again', 'call-2'),
      result('r2', 'call-2', 'BBB'),
    ])

    expect(model.visibleEntries.filter((entry) => entry.kind === 'tool_call')).toHaveLength(2)
    // Results never render on their own when their call is on screen.
    expect(model.visibleEntries.filter((entry) => entry.kind === 'tool_result')).toHaveLength(0)
    for (const entry of model.visibleEntries) {
      if (entry.kind === 'tool_call') expect(model.resultForCall(entry)).toBeDefined()
    }
  })

  it('labels a pending call as running only while the turn is live', () => {
    expect(pendingToolLabel(true)).toBe('运行中…')
    expect(pendingToolLabel(false)).toBe('未完成')
  })

  it('marks a settled step group as unfinished rather than running', () => {
    const entries = [call('c1', 'call-1')]
    const model = buildLogModel(entries)
    expect(stepSummary(entries, model.resultForCall, false)).toContain('未完成')
    expect(stepSummary(entries, model.resultForCall, false)).not.toContain('运行中')
    expect(stepSummary(entries, model.resultForCall, true)).toContain('运行中')
  })

  it('shows a pending call as unfinished once the turn ended', async () => {
    const wrapper = mount(MessageView, {
      props: { messages: [call('c1', 'call-1')], capabilities: capabilities(), running: false },
    })
    const block = wrapper.find('[data-test="tool-call"]')
    expect(block.exists()).toBe(true)
    await block.find('summary').trigger('click')
    expect(block.text()).toContain('未完成')
    expect(block.text()).not.toContain('运行中…')
  })

  it('keeps the running label while the turn is live', async () => {
    const wrapper = mount(MessageView, {
      props: { messages: [call('c1', 'call-1')], capabilities: capabilities(), running: true },
    })
    const block = wrapper.find('[data-test="tool-call"]')
    await block.find('summary').trigger('click')
    expect(block.text()).toContain('运行中…')
  })
})

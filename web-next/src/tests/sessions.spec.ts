import { beforeEach, describe, expect, it } from 'vitest'
import type { AgentSession } from '../api/sessions'
import { consumeFrame, useSessionsStore } from '../stores/sessions'

const capabilities = {
  streaming: true,
  sessionDiscovery: true,
  sessionContinue: true,
  stop: true,
  interrupt: true,
  modelSelection: true,
  toolCallEvents: true,
  toolConfirmation: true,
  sessionHistory: true,
  rawEvents: false,
}

const session: AgentSession = {
  id: 's1',
  projectId: 1,
  backend: 'pi',
  instanceId: 'i1',
  nativeSessionId: 'n1',
  title: 'hello',
  model: 'pi/gpt-5',
  status: 'active',
  turn: 'idle',
  capabilities,
  createdAt: '2026-10-24T00:00:00Z',
  updatedAt: '2026-10-24T00:00:00Z',
}

function reset() {
  useSessionsStore.setState({
    sessions: [session],
    sessionsByProject: { 1: [session] },
    sessionTurns: { s1: 'idle' },
    messages: {},
    streamStatus: {},
    historyLoaded: {},
    historyError: {},
    historyTruncated: {},
    historyTailGuard: {},
    echoPending: {},
    selectedModels: {},
    autoModels: {},
    activeSessionId: null,
    lifecycleUsage: {},
    loading: false,
    error: '',
    errorCode: '',
    listRequest: 0,
    revision: 0,
  })
}

describe('session stream entries', () => {
  beforeEach(reset)

  it('merges deltas into one assistant message and settles on final', () => {
    consumeFrame(
      's1',
      JSON.stringify({
        type: 'message',
        sequence: 1,
        message: { role: 'assistant', text: 'Hel', final: false },
      }),
    )
    consumeFrame(
      's1',
      JSON.stringify({
        type: 'message',
        sequence: 2,
        message: { role: 'assistant', text: 'Hello', final: true },
      }),
    )

    const entries = useSessionsStore.getState().messages.s1
    expect(entries).toHaveLength(1)
    expect(entries[0].text).toBe('Hello')
    expect(entries[0].streaming).toBe(false)
  })

  it('drops an echoed user message instead of duplicating it', () => {
    useSessionsStore.setState({ echoPending: { s1: 'hello' } })
    consumeFrame(
      's1',
      JSON.stringify({
        type: 'message',
        sequence: 1,
        message: { role: 'user', text: 'hello', final: true },
      }),
    )
    expect(useSessionsStore.getState().messages.s1).toBeUndefined()
    expect(useSessionsStore.getState().echoPending.s1).toBeUndefined()
  })

  it('turns a started event into running state and a finished event settles entries', () => {
    useSessionsStore.getState().appendMessage('s1', 'assistant', 'partial', false, 1)
    consumeFrame('s1', JSON.stringify({ type: 'turn', turn: { phase: 'started' } }))
    expect(useSessionsStore.getState().sessionTurns.s1).toBe('running')

    consumeFrame('s1', JSON.stringify({ type: 'turn', turn: { phase: 'ended', outcome: 'completed' } }))
    expect(useSessionsStore.getState().sessionTurns.s1).toBe('idle')
    expect(useSessionsStore.getState().messages.s1?.[0].streaming).toBe(false)
  })
})

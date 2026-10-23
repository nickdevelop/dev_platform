// Contract tests for the frontend API client: request bodies and paths must
// match the backend routes and docs/api/openapi.yaml exactly, so a drift in the
// wire format fails here rather than in an integration no test covers.
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createProjectSession, getSessionHistory, interruptProjectSession } from '../api/sessions'
import { listAgentBackendModels, refreshAgentBackendModels } from '../api/agents'
import { getProjectGitInfo } from '../api/git'
import { createService, fetchModels, testModel } from '../api/models'
import { getAppearance, listAgents, putAppearance, setAgentEnabled } from '../api/settings'
import { installTestCSRF, testCSRFToken } from './csrf-fixture'

afterEach(() => {
  vi.unstubAllGlobals()
})

interface Call {
  url: string
  init: RequestInit | undefined
}

function stubJson(payload: unknown, status = 200): Call[] {
  const calls: Call[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push({ url: String(input), init })
      return { status, json: async () => payload } as Response
    }),
  )
  return calls
}

function headersOf(call: Call): Record<string, string> {
  return (call.init?.headers ?? {}) as Record<string, string>
}

describe('session API contracts', () => {
  it('creates a session with { backend, title } and never sends instanceId', async () => {
    installTestCSRF()
    const calls = stubJson({ id: 's1' })

    await createProjectSession(7, { backend: 'pi', title: 'Hello' })

    expect(calls).toHaveLength(1)
    expect(calls[0].url).toBe('/api/projects/7/sessions')
    expect(calls[0].init?.method).toBe('POST')
    const body = JSON.parse(String(calls[0].init?.body))
    expect(body).toEqual({ backend: 'pi', title: 'Hello' })
    expect(body).not.toHaveProperty('instanceId')
    expect(headersOf(calls[0])['X-CSRF-Token']).toBe(testCSRFToken)
  })

  it('interrupts a session through the interrupt route', async () => {
    installTestCSRF()
    const calls = stubJson({ id: 's1', turn: 'idle' })

    await interruptProjectSession(7, 'ses/1')

    expect(calls[0].url).toBe('/api/projects/7/sessions/ses%2F1/interrupt')
    expect(calls[0].init?.method).toBe('POST')
    expect(headersOf(calls[0])['X-CSRF-Token']).toBe(testCSRFToken)
  })
})

describe('agents API contracts', () => {
  it('reads the platform model snapshot from the backend models route', async () => {
    const calls = stubJson({
      models: [{ id: 'pi/gpt', name: 'gpt', provider: 'pi' }],
      updatedAt: '2026-10-13T12:00:00Z',
    })

    const result = await listAgentBackendModels('pi/1')

    expect(calls[0].url).toBe('/api/agent-backends/pi%2F1/models')
    expect(calls[0].init?.method).toBe('GET')
    expect(result.models[0].id).toBe('pi/gpt')
    expect(result.updatedAt).toBe('2026-10-13T12:00:00Z')
  })

  it('refreshes the platform model snapshot through the POST route', async () => {
    installTestCSRF()
    const calls = stubJson({ models: [], updatedAt: '2026-10-13T12:00:00Z' })

    await refreshAgentBackendModels('pi')

    expect(calls[0].url).toBe('/api/agent-backends/pi/models/refresh')
    expect(calls[0].init?.method).toBe('POST')
    expect(headersOf(calls[0])['X-CSRF-Token']).toBe(testCSRFToken)
  })

  it('reads project git state from the project git-info route', async () => {
    const calls = stubJson({ isGit: true, branch: 'main', dirty: false })

    const info = await getProjectGitInfo(7)

    expect(calls).toHaveLength(1)
    expect(calls[0].url).toBe('/api/projects/7/git-info')
    expect(calls[0].init?.method).toBe('GET')
    expect(info.branch).toBe('main')
  })
})

describe('session history API contract', () => {
  it('reads history from the session history route without a CSRF token', async () => {
    const calls = stubJson({ events: [], truncated: true })

    const history = await getSessionHistory(7, 'ses/1')

    expect(calls).toHaveLength(1)
    expect(calls[0].url).toBe('/api/projects/7/sessions/ses%2F1/history')
    expect(calls[0].init?.method).toBe('GET')
    expect(headersOf(calls[0])['X-CSRF-Token']).toBeUndefined()
    expect(history.truncated).toBe(true)
  })
})

describe('settings and model API contracts', () => {
  it('reads and writes appearance at /settings/appearance', async () => {
    installTestCSRF()
    const calls = stubJson({})

    await getAppearance()
    expect(calls[0].url).toBe('/api/settings/appearance')
    expect(calls[0].init?.method).toBe('GET')
    expect(headersOf(calls[0])['X-CSRF-Token']).toBeUndefined()

    await putAppearance({ theme: 'light', fontSize: 14, density: 'standard' })
    expect(calls[1].url).toBe('/api/settings/appearance')
    expect(calls[1].init?.method).toBe('PUT')
    expect(headersOf(calls[1])['X-CSRF-Token']).toBe(testCSRFToken)
  })

  it('lists agents and toggles platform enablement', async () => {
    installTestCSRF()
    const calls = stubJson({})

    await listAgents()
    expect(calls[0].url).toBe('/api/agents')

    await setAgentEnabled('pi', false)
    expect(calls[1].url).toBe('/api/agents/pi')
    expect(calls[1].init?.method).toBe('PUT')
    expect(headersOf(calls[1])['X-CSRF-Token']).toBe(testCSRFToken)
    expect(JSON.parse(String(calls[1].init?.body))).toEqual({ enabled: false })
  })

  it('creates a service, fetches models and tests a model', async () => {
    installTestCSRF()
    const calls = stubJson({})

    await createService({ name: 's', baseUrl: 'https://x/v1', apiKey: 'k', enabled: true })
    expect(calls[0].url).toBe('/api/model-services')
    expect(calls[0].init?.method).toBe('POST')
    expect(headersOf(calls[0])['X-CSRF-Token']).toBe(testCSRFToken)

    // Both probes are POSTs that the transport must sign: a regression here is
    // exactly the 403 invalid_csrf_token this contract was added for.
    await fetchModels({ baseUrl: 'https://x/v1', apiKey: 'k' })
    expect(calls[1].url).toBe('/api/model-services/fetch-models')
    expect(calls[1].init?.method).toBe('POST')
    expect(headersOf(calls[1])['X-CSRF-Token']).toBe(testCSRFToken)

    await testModel({ baseUrl: 'https://x/v1', modelId: 'm' })
    expect(calls[2].url).toBe('/api/models/test')
    expect(calls[2].init?.method).toBe('POST')
    expect(headersOf(calls[2])['X-CSRF-Token']).toBe(testCSRFToken)
  })
})

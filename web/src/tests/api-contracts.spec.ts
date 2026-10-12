// Contract tests for the frontend API client: request bodies and paths must
// match the backend routes and docs/api/openapi.yaml exactly, so a drift in the
// wire format fails here rather than in an integration no test covers.
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createProjectSession, getSessionHistory, interruptProjectSession } from '../api/sessions'
import { listInstanceModels } from '../api/agents'
import { createService, fetchModels, testModel } from '../api/models'
import { getAppearance, listAgents, putAppearance, setAgentEnabled } from '../api/settings'

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
    const calls = stubJson({ id: 's1' })

    await createProjectSession(7, { backend: 'pi', title: 'Hello' }, 'csrf-1')

    expect(calls).toHaveLength(1)
    expect(calls[0].url).toBe('/api/projects/7/sessions')
    expect(calls[0].init?.method).toBe('POST')
    const body = JSON.parse(String(calls[0].init?.body))
    expect(body).toEqual({ backend: 'pi', title: 'Hello' })
    expect(body).not.toHaveProperty('instanceId')
    expect(headersOf(calls[0])['X-CSRF-Token']).toBe('csrf-1')
  })

  it('interrupts a session through the interrupt route', async () => {
    const calls = stubJson({ id: 's1', turn: 'idle' })

    await interruptProjectSession(7, 'ses/1', 'csrf-2')

    expect(calls[0].url).toBe('/api/projects/7/sessions/ses%2F1/interrupt')
    expect(calls[0].init?.method).toBe('POST')
    expect(headersOf(calls[0])['X-CSRF-Token']).toBe('csrf-2')
  })
})

describe('agents API contracts', () => {
  it('lists an instance models from the instance models route', async () => {
    const calls = stubJson({ models: [{ id: 'pi/gpt', name: 'gpt', provider: 'pi' }] })

    const result = await listInstanceModels(7, 'inst/1')

    expect(calls[0].url).toBe('/api/projects/7/instances/inst%2F1/models')
    expect(calls[0].init?.method).toBe('GET')
    expect(result.models[0].id).toBe('pi/gpt')
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
    const calls = stubJson({})

    await getAppearance()
    expect(calls[0].url).toBe('/api/settings/appearance')
    expect(calls[0].init?.method).toBe('GET')

    await putAppearance(
      { theme: 'light', font: 'system', customFont: '', fontSize: 14, density: 'standard' },
      'csrf-1',
    )
    expect(calls[1].url).toBe('/api/settings/appearance')
    expect(calls[1].init?.method).toBe('PUT')
    expect(headersOf(calls[1])['X-CSRF-Token']).toBe('csrf-1')
  })

  it('lists agents and toggles platform enablement', async () => {
    const calls = stubJson({})

    await listAgents()
    expect(calls[0].url).toBe('/api/agents')

    await setAgentEnabled('pi', false, 'csrf-2')
    expect(calls[1].url).toBe('/api/agents/pi')
    expect(calls[1].init?.method).toBe('PUT')
    expect(JSON.parse(String(calls[1].init?.body))).toEqual({ enabled: false })
  })

  it('creates a service, fetches models and tests a model', async () => {
    const calls = stubJson({})

    await createService({ name: 's', baseUrl: 'https://x/v1', apiKey: 'k', enabled: true }, 'csrf-3')
    expect(calls[0].url).toBe('/api/model-services')
    expect(calls[0].init?.method).toBe('POST')

    await fetchModels({ baseUrl: 'https://x/v1', apiKey: 'k' })
    expect(calls[1].url).toBe('/api/model-services/fetch-models')

    await testModel({ baseUrl: 'https://x/v1', modelId: 'm' })
    expect(calls[2].url).toBe('/api/models/test')
  })
})

// Contract tests for the Team Groups & Group Tasks API module:
// verifies HTTP verbs, URLs, JSON bodies, and CSRF token transmission.
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  cancelGroupRun,
  createGroup,
  createGroupTask,
  deleteGroup,
  deleteGroupTask,
  getGroup,
  getGroupTask,
  listGroups,
  listGroupTaskRuns,
  listGroupRunMessages,
  listGroupTasks,
  postGroupRunMessage,
  setGroupMembers,
  triggerGroupTask,
  updateGroup,
  updateGroupTask,
} from '../api/groups'
import { installTestCSRF, testCSRFToken } from './csrf-fixture'

interface Call {
  url: string
  init?: RequestInit
}

function stub(payload: unknown = {}): Call[] {
  const calls: Call[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push({ url: String(input), init })
      return {
        status: 200,
        json: async () => payload,
      } as Response
    }),
  )
  return calls
}

function headers(call: Call): Record<string, string> {
  return (call.init?.headers ?? {}) as Record<string, string>
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('groups API contracts', () => {
  it('reads groups and single group without CSRF header', async () => {
    const calls = stub({ groups: [] })
    await listGroups()
    await getGroup(42)

    expect(calls[0].url).toBe('/api/groups')
    expect(calls[0].init?.method).toBe('GET')
    expect(headers(calls[0])['X-CSRF-Token']).toBeUndefined()

    expect(calls[1].url).toBe('/api/groups/42')
    expect(calls[1].init?.method).toBe('GET')
    expect(headers(calls[1])['X-CSRF-Token']).toBeUndefined()
  })

  it('creates a group with POST /api/groups and passes CSRF token and JSON body', async () => {
    installTestCSRF()
    const calls = stub({ id: 1, name: 'Architecture Team' })
    const payload = {
      name: 'Architecture Team',
      description: 'System redesign',
      orchestratorModel: 'claude-3-7-sonnet',
      orchestratorPrompt: 'Coordinate agents',
      roleIds: [10, 20],
    }

    const res = await createGroup(payload)
    expect(res).toEqual({ id: 1, name: 'Architecture Team' })
    expect(calls).toHaveLength(1)
    expect(calls[0].url).toBe('/api/groups')
    expect(calls[0].init?.method).toBe('POST')
    expect(headers(calls[0])['X-CSRF-Token']).toBe(testCSRFToken)
    expect(headers(calls[0])['Content-Type']).toBe('application/json')
    expect(JSON.parse(String(calls[0].init?.body))).toEqual(payload)
  })

  it('updates a group with PUT /api/groups/:id', async () => {
    installTestCSRF()
    const calls = stub({ id: 2, name: 'Updated Team' })
    const payload = {
      name: 'Updated Team',
      orchestratorModel: 'gpt-4o',
    }

    await updateGroup(2, payload)
    expect(calls).toHaveLength(1)
    expect(calls[0].url).toBe('/api/groups/2')
    expect(calls[0].init?.method).toBe('PUT')
    expect(headers(calls[0])['X-CSRF-Token']).toBe(testCSRFToken)
    expect(JSON.parse(String(calls[0].init?.body))).toEqual(payload)
  })

  it('deletes a group with DELETE /api/groups/:id', async () => {
    installTestCSRF()
    const calls = stub()
    await deleteGroup(3)

    expect(calls).toHaveLength(1)
    expect(calls[0].url).toBe('/api/groups/3')
    expect(calls[0].init?.method).toBe('DELETE')
    expect(headers(calls[0])['X-CSRF-Token']).toBe(testCSRFToken)
  })

  it('sets group members with PUT /api/groups/:id/members', async () => {
    installTestCSRF()
    const calls = stub({ members: [{ id: 1, groupId: 5, roleId: 8 }] })
    const members = await setGroupMembers(5, [8, 9])

    expect(members).toEqual([{ id: 1, groupId: 5, roleId: 8 }])
    expect(calls).toHaveLength(1)
    expect(calls[0].url).toBe('/api/groups/5/members')
    expect(calls[0].init?.method).toBe('PUT')
    expect(headers(calls[0])['X-CSRF-Token']).toBe(testCSRFToken)
    expect(JSON.parse(String(calls[0].init?.body))).toEqual({ roleIds: [8, 9] })
  })

  it('reads group tasks and task runs without CSRF', async () => {
    const calls = stub({ tasks: [], runs: [] })
    await listGroupTasks(7)
    await getGroupTask(7, 101)
    await listGroupTaskRuns(7, 101)
    await listGroupRunMessages(55)

    expect(calls[0].url).toBe('/api/groups/7/tasks')
    expect(calls[0].init?.method).toBe('GET')
    expect(headers(calls[0])['X-CSRF-Token']).toBeUndefined()

    expect(calls[1].url).toBe('/api/groups/7/tasks/101')
    expect(calls[1].init?.method).toBe('GET')

    expect(calls[2].url).toBe('/api/groups/7/tasks/101/runs')
    expect(calls[2].init?.method).toBe('GET')

    expect(calls[3].url).toBe('/api/groups/runs/55/messages')
    expect(calls[3].init?.method).toBe('GET')
    expect(headers(calls[3])['X-CSRF-Token']).toBeUndefined()
  })

  it('creates, updates, and deletes group tasks with correct methods and bodies', async () => {
    installTestCSRF()
    const calls = stub({ id: 99, title: 'Refactor Auth' })

    const createPayload = {
      title: 'Refactor Auth',
      description: 'Migrate to JWT',
      projectId: 1,
      worktreeId: 0,
      prompt: 'Do auth refactor',
      scheduleType: 'manual' as const,
    }
    await createGroupTask(7, createPayload)
    expect(calls[0].url).toBe('/api/groups/7/tasks')
    expect(calls[0].init?.method).toBe('POST')
    expect(headers(calls[0])['X-CSRF-Token']).toBe(testCSRFToken)
    expect(JSON.parse(String(calls[0].init?.body))).toEqual(createPayload)

    await updateGroupTask(7, 99, { title: 'Renamed Auth' })
    expect(calls[1].url).toBe('/api/groups/7/tasks/99')
    expect(calls[1].init?.method).toBe('PUT')
    expect(headers(calls[1])['X-CSRF-Token']).toBe(testCSRFToken)
    expect(JSON.parse(String(calls[1].init?.body))).toEqual({ title: 'Renamed Auth' })

    await deleteGroupTask(7, 99)
    expect(calls[2].url).toBe('/api/groups/7/tasks/99')
    expect(calls[2].init?.method).toBe('DELETE')
    expect(headers(calls[2])['X-CSRF-Token']).toBe(testCSRFToken)
  })

  it('triggers a group task run with prompt', async () => {
    installTestCSRF()
    const calls = stub({ id: 10, taskId: 99, status: 'running' })
    await triggerGroupTask(7, 99, 'Start execution now')

    expect(calls).toHaveLength(1)
    expect(calls[0].url).toBe('/api/groups/7/tasks/99/trigger')
    expect(calls[0].init?.method).toBe('POST')
    expect(headers(calls[0])['X-CSRF-Token']).toBe(testCSRFToken)
    expect(JSON.parse(String(calls[0].init?.body))).toEqual({ prompt: 'Start execution now' })
  })

  it('posts run message and cancels group run', async () => {
    installTestCSRF()
    const calls = stub({ id: 100, runId: 10, content: 'User instruction' })

    await postGroupRunMessage(10, 'User instruction')
    expect(calls[0].url).toBe('/api/groups/runs/10/messages')
    expect(calls[0].init?.method).toBe('POST')
    expect(headers(calls[0])['X-CSRF-Token']).toBe(testCSRFToken)
    expect(JSON.parse(String(calls[0].init?.body))).toEqual({ content: 'User instruction' })

    await cancelGroupRun(10)
    expect(calls[1].url).toBe('/api/groups/runs/10/cancel')
    expect(calls[1].init?.method).toBe('POST')
    expect(headers(calls[1])['X-CSRF-Token']).toBe(testCSRFToken)
  })
})

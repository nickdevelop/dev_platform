// Unit tests for the opencode raw approval stream: permission/form events
// project onto pending state, foreign sessions are dropped, and replies hit
// the /opencode prefix with the desktop decision values.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useOpencodeRawStore } from '../stores/opencodeRaw'
import * as opencodeApi from '../api/opencode'

describe('opencodeRaw store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.unstubAllGlobals()
  })

  it('projects permission.asked/replied onto pending approvals', () => {
    const store = useOpencodeRawStore()
    store.handleRawEvent('sess-1', 'native-1', {
      type: 'permission.asked',
      data: { sessionID: 'native-1', requestID: 'per_1', action: 'read' },
    } as never)
    expect(store.pendingApprovalFor('sess-1')).toEqual({ requestId: 'per_1', action: 'read' })

    store.handleRawEvent('sess-1', 'native-1', {
      type: 'permission.replied',
      data: { sessionID: 'native-1', requestID: 'per_1' },
    } as never)
    expect(store.pendingApprovalFor('sess-1')).toBeNull()
  })

  it('drops events for other sessions', () => {
    const store = useOpencodeRawStore()
    store.handleRawEvent('sess-1', 'native-1', {
      type: 'permission.asked',
      data: { sessionID: 'native-other', requestID: 'per_x', action: 'read' },
    } as never)
    expect(store.pendingApprovalFor('sess-1')).toBeNull()
  })

  it('projects form.created/replied onto pending forms', () => {
    const store = useOpencodeRawStore()
    store.handleRawEvent('sess-1', 'native-1', {
      type: 'form.created',
      data: { sessionID: 'native-1', form: { id: 'frm_1', title: 'confirm' } },
    } as never)
    expect(store.formsFor('sess-1')).toHaveLength(1)
    store.handleRawEvent('sess-1', 'native-1', {
      type: 'form.replied',
      data: { sessionID: 'native-1', id: 'frm_1' },
    } as never)
    expect(store.formsFor('sess-1')).toHaveLength(0)
  })

  it('keeps form fields for the question card', () => {
    const store = useOpencodeRawStore()
    const fields = [{ key: 'choice', type: 'string', title: 'pick', options: [{ value: 'a', label: 'A' }] }]
    store.handleRawEvent('sess-1', 'native-1', {
      type: 'form.created',
      data: { sessionID: 'native-1', form: { id: 'frm_1', title: 'q', fields } },
    } as never)
    expect(store.formsFor('sess-1')[0].fields).toEqual(fields)
  })

  it('replies with once/reject on the opencode prefix', async () => {
    const reply = vi.spyOn(opencodeApi, 'replyOpencodePermission').mockResolvedValue(undefined)
    const store = useOpencodeRawStore()
    store.permissions['sess-1'] = [
      { id: 'per_1', sessionId: 'sess-1', action: 'read', resources: [] },
    ]
    const ok = await store.replyPermission(1, 'sess-1', 'per_1', 'once')
    expect(ok).toBe(true)
    expect(reply).toHaveBeenCalledWith(1, 'sess-1', 'per_1', { decision: 'once' }, expect.any(String))
    expect(store.permissionsFor('sess-1')).toHaveLength(0)
  })

  it('drops the stale control when the server reports an expired reply', async () => {
    const { ApiError } = await import('../api/client')
    vi.spyOn(opencodeApi, 'replyOpencodePermission').mockRejectedValue(
      new ApiError(409, 'permission_expired'),
    )
    const store = useOpencodeRawStore()
    store.permissions['sess-1'] = [
      { id: 'per_old', sessionId: 'sess-1', action: 'read', resources: [] },
    ]
    const ok = await store.replyPermission(1, 'sess-1', 'per_old', 'once')
    expect(ok).toBe(false)
    expect(store.permissionsFor('sess-1')).toHaveLength(0)
    expect(store.error).toContain('过期')
  })

  it('builds the raw events path under the opencode prefix', () => {
    expect(opencodeApi.opencodeRawEventsPath(7, 'sess-1')).toBe(
      '/api/projects/7/opencode/sessions/sess-1/raw-events',
    )
  })
})

// Unit tests for the pending-interaction store: requested and resolved events
// from the session stream project onto pending state, the stream's snapshot
// replaces it, and replies hit the generic session interaction routes with
// the desktop decision values.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useInteractionsStore } from '../stores/interactions'
import * as interactionsApi from '../api/interactions'
import { installTestCSRF } from './csrf-fixture'

// The transport refuses mutating requests without a token source; the app
// installs the real provider in main.ts, which these specs never run.
installTestCSRF()

describe('interactions store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.unstubAllGlobals()
  })

  it('adds a requested permission and removes it when resolved', () => {
    const store = useInteractionsStore()
    store.applyInteraction('sess-1', {
      id: 'per_1',
      kind: 'permission',
      phase: 'requested',
      permission: { id: 'per_1', sessionId: 'native-1', action: 'read', resources: [] },
    })
    expect(store.pendingApprovalFor('sess-1')).toEqual({ requestId: 'per_1', action: 'read' })

    store.applyInteraction('sess-1', { id: 'per_1', kind: 'permission', phase: 'resolved' })
    expect(store.pendingApprovalFor('sess-1')).toBeNull()
  })

  it('does not duplicate a request re-sent by a resumed stream', () => {
    const store = useInteractionsStore()
    const requested = {
      id: 'per_1',
      kind: 'permission' as const,
      phase: 'requested' as const,
      permission: { id: 'per_1', sessionId: 'native-1', action: 'read', resources: [] },
    }
    store.applyInteraction('sess-1', requested)
    store.applyInteraction('sess-1', requested)
    expect(store.permissionsFor('sess-1')).toHaveLength(1)
  })

  it('keeps sessions apart', () => {
    const store = useInteractionsStore()
    store.applyInteraction('sess-1', {
      id: 'per_1',
      kind: 'permission',
      phase: 'requested',
      permission: { id: 'per_1', sessionId: 'native-1', action: 'read', resources: [] },
    })
    expect(store.pendingApprovalFor('sess-2')).toBeNull()
  })

  it('adds a requested form and removes it when resolved', () => {
    const store = useInteractionsStore()
    store.applyInteraction('sess-1', {
      id: 'frm_1',
      kind: 'form',
      phase: 'requested',
      form: { id: 'frm_1', sessionId: 'native-1', title: 'confirm' },
    })
    expect(store.formsFor('sess-1')).toHaveLength(1)
    store.applyInteraction('sess-1', { id: 'frm_1', kind: 'form', phase: 'resolved' })
    expect(store.formsFor('sess-1')).toHaveLength(0)
  })

  it('keeps form fields for the question card', () => {
    const store = useInteractionsStore()
    const fields = [{ key: 'choice', type: 'string', title: 'pick', options: [{ value: 'a', label: 'A' }] }]
    store.applyInteraction('sess-1', {
      id: 'frm_1',
      kind: 'form',
      phase: 'requested',
      form: { id: 'frm_1', sessionId: 'native-1', title: 'q', fields },
    })
    expect(store.formsFor('sess-1')[0].fields).toEqual(fields)
  })

  it('replaces the pending set with the snapshot a stream opens with', () => {
    const store = useInteractionsStore()
    store.applyInteraction('sess-1', {
      id: 'per_stale',
      kind: 'permission',
      phase: 'requested',
      permission: { id: 'per_stale', sessionId: 'native-1', action: 'read', resources: [] },
    })
    store.applySnapshot('sess-1', {
      permissions: [{ id: 'per_1', sessionId: 'native-1', action: 'bash', resources: ['ls'] }],
      forms: [],
    })
    expect(store.permissionsFor('sess-1').map((entry) => entry.id)).toEqual(['per_1'])
    expect(store.formsFor('sess-1')).toEqual([])
  })

  it('replies with once/reject on the interaction prefix', async () => {
    const reply = vi.spyOn(interactionsApi, 'replyPermissionRequest').mockResolvedValue(undefined)
    const store = useInteractionsStore()
    store.permissions['sess-1'] = [
      { id: 'per_1', sessionId: 'sess-1', action: 'read', resources: [] },
    ]
    const ok = await store.replyPermission(1, 'sess-1', 'per_1', 'once')
    expect(ok).toBe(true)
    expect(reply).toHaveBeenCalledWith(1, 'sess-1', 'per_1', { decision: 'once' })
    expect(store.permissionsFor('sess-1')).toHaveLength(0)
  })

  it('drops the stale control when the server reports an expired reply', async () => {
    const { ApiError } = await import('../api/client')
    vi.spyOn(interactionsApi, 'replyPermissionRequest').mockRejectedValue(
      new ApiError(409, 'permission_expired'),
    )
    const store = useInteractionsStore()
    store.permissions['sess-1'] = [
      { id: 'per_old', sessionId: 'sess-1', action: 'read', resources: [] },
    ]
    const ok = await store.replyPermission(1, 'sess-1', 'per_old', 'once')
    expect(ok).toBe(false)
    expect(store.permissionsFor('sess-1')).toHaveLength(0)
    expect(store.error).toContain('过期')
  })

  it('builds the raw events path under the session path', () => {
    expect(interactionsApi.rawEventsPath(7, 'sess-1')).toBe(
      '/api/projects/7/sessions/sess-1/raw-events',
    )
  })
})

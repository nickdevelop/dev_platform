import { describe, expect, it, vi } from 'vitest'
import { computed, nextTick, ref } from 'vue'
import { useAutoApproval } from '../features/chat/interactions/useAutoApproval'
import type { ApprovalMode } from '../features/chat/interactions/autoApproval'
import type { FormInfo } from '../api/interactions'

describe('useAutoApproval', () => {
  it('does nothing in default mode', async () => {
    const mode = ref<ApprovalMode>('default')
    const pendingTool = ref<{ callId: string; name: string } | null>({ callId: 'call_1', name: 'bash' })
    const pendingForms = ref<FormInfo[]>([])
    const confirmTool = vi.fn()
    const answerForm = vi.fn()

    useAutoApproval({
      mode,
      pendingToolCall: computed(() => pendingTool.value),
      pendingForms: computed(() => pendingForms.value),
      confirmTool,
      answerForm,
    })

    await nextTick()
    expect(confirmTool).not.toHaveBeenCalled()
  })

  it('automatically approves tool in auto-approve mode', async () => {
    const mode = ref<ApprovalMode>('auto-approve')
    const pendingTool = ref<{ callId: string; name: string } | null>(null)
    const pendingForms = ref<FormInfo[]>([])
    const confirmTool = vi.fn()
    const answerForm = vi.fn()

    useAutoApproval({
      mode,
      pendingToolCall: computed(() => pendingTool.value),
      pendingForms: computed(() => pendingForms.value),
      confirmTool,
      answerForm,
    })

    await nextTick()
    expect(confirmTool).not.toHaveBeenCalled()

    // Tool call arrives
    pendingTool.value = { callId: 'call_1', name: 'bash' }
    await nextTick()

    expect(confirmTool).toHaveBeenCalledWith('call_1')
    expect(confirmTool).toHaveBeenCalledTimes(1)

    // Should not call again for the same callId
    pendingTool.value = { callId: 'call_1', name: 'bash' }
    await nextTick()
    expect(confirmTool).toHaveBeenCalledTimes(1)
  })

  it('triggers approval when switching mode to auto-approve while a call is pending', async () => {
    const mode = ref<ApprovalMode>('default')
    const pendingTool = ref<{ callId: string; name: string } | null>({ callId: 'call_pending', name: 'git' })
    const pendingForms = ref<FormInfo[]>([])
    const confirmTool = vi.fn()
    const answerForm = vi.fn()

    useAutoApproval({
      mode,
      pendingToolCall: computed(() => pendingTool.value),
      pendingForms: computed(() => pendingForms.value),
      confirmTool,
      answerForm,
    })

    await nextTick()
    expect(confirmTool).not.toHaveBeenCalled()

    mode.value = 'auto-approve'
    await nextTick()
    expect(confirmTool).toHaveBeenCalledWith('call_pending')
  })

  it('in autopilot mode, automatically approves tools and submits forms with recommended answers', async () => {
    const mode = ref<ApprovalMode>('autopilot')
    const pendingTool = ref<{ callId: string; name: string } | null>({ callId: 'call_cmd', name: 'bash' })
    const pendingForms = ref<FormInfo[]>([
      {
        id: 'form_1',
        sessionId: 'ses_1',
        title: 'Choice',
        fields: [
          {
            key: 'target',
            type: 'string',
            required: true,
            options: [
              { value: 'v1', label: 'Version 1' },
              { value: 'v2', label: 'Version 2 (Recommended)' },
            ],
          },
        ],
      },
    ])
    const confirmTool = vi.fn()
    const answerForm = vi.fn()

    useAutoApproval({
      mode,
      pendingToolCall: computed(() => pendingTool.value),
      pendingForms: computed(() => pendingForms.value),
      confirmTool,
      answerForm,
    })

    await nextTick()
    expect(confirmTool).toHaveBeenCalledWith('call_cmd')
    expect(answerForm).toHaveBeenCalledWith('form_1', { target: 'v2' })
  })

  it('in autopilot mode, does not auto-submit if required fields cannot be resolved', async () => {
    const mode = ref<ApprovalMode>('autopilot')
    const pendingTool = ref<{ callId: string; name: string } | null>(null)
    const pendingForms = ref<FormInfo[]>([
      {
        id: 'form_open_ended',
        sessionId: 'ses_1',
        title: 'Input needed',
        fields: [
          {
            key: 'text',
            type: 'string',
            required: true,
          },
        ],
      },
    ])
    const confirmTool = vi.fn()
    const answerForm = vi.fn()

    useAutoApproval({
      mode,
      pendingToolCall: computed(() => pendingTool.value),
      pendingForms: computed(() => pendingForms.value),
      confirmTool,
      answerForm,
    })

    await nextTick()
    expect(answerForm).not.toHaveBeenCalled()
  })
})

import { watch, type ComputedRef, type Ref } from 'vue'
import type { FormInfo } from '../../../api/interactions'
import { resolveAutopilotForm, type ApprovalMode } from './autoApproval'

export interface AutoApprovalOptions {
  mode: Ref<ApprovalMode>
  pendingToolCall: ComputedRef<{ callId: string; name: string } | null>
  pendingForms: ComputedRef<FormInfo[]>
  confirmTool: (callId: string) => void
  answerForm: (formId: string, answer: Record<string, unknown>) => Promise<void> | void
}

export function useAutoApproval(options: AutoApprovalOptions) {
  const handledToolCalls = new Set<string>()
  const handledForms = new Set<string>()

  function tryAutoApproveTool() {
    if (options.mode.value === 'default') return
    const call = options.pendingToolCall.value
    if (!call || !call.callId) return
    if (handledToolCalls.has(call.callId)) return

    handledToolCalls.add(call.callId)
    options.confirmTool(call.callId)
  }

  function tryAutopilotForms() {
    if (options.mode.value !== 'autopilot') return
    const forms = options.pendingForms.value
    if (!forms || forms.length === 0) return

    for (const form of forms) {
      if (!form || !form.id) continue
      if (handledForms.has(form.id)) continue

      const resolution = resolveAutopilotForm(form)
      if (resolution.canAutoSubmit) {
        handledForms.add(form.id)
        void options.answerForm(form.id, resolution.answers)
      }
    }
  }

  watch(
    () => [options.mode.value, options.pendingToolCall.value],
    () => {
      tryAutoApproveTool()
    },
    { immediate: true, flush: 'post' },
  )

  watch(
    () => [options.mode.value, options.pendingForms.value],
    () => {
      tryAutopilotForms()
    },
    { immediate: true, flush: 'post' },
  )

  return {
    tryAutoApproveTool,
    tryAutopilotForms,
  }
}

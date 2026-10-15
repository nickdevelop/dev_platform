// The chat feature's interaction layer for one open session: which tool call
// awaits confirmation, which question forms are pending, and how a decision
// travels back to the backend. Approval push arrives through the raw event
// stream (capability-gated); a backend with tool confirmation but no push
// channel (pi over RPC) is polled while a turn runs and stops when it settles.

import { computed, onBeforeUnmount, ref, watch, type ComputedRef, type Ref } from 'vue'
import type { AgentSession } from '../../../api/sessions'
import type { ChatEntry } from '../state/sessionLog'
import { useInteractionsStore } from '../../../stores/interactions'

export interface SessionInteractionOptions {
  projectId: Ref<number>
  sessionId: Ref<string>
  activeSession: ComputedRef<AgentSession | null>
  activeMessages: ComputedRef<ChatEntry[]>
  isRunning: ComputedRef<boolean>
  // Fired after a form answer is accepted: the composer's column re-pins to
  // the newest entry, mirroring a normal turn's end.
  afterFormAnswer: () => void
}

export function useSessionInteractions(options: SessionInteractionOptions) {
  const interactions = useInteractionsStore()

  const toolNotice = ref('')

  // The tool call awaiting confirmation. A backend that declares tool
  // confirmation gets it exclusively from its approval stream (REST list + raw
  // permission events): a tool_call entry without a result is NOT a permission —
  // replying with a tool call id can never succeed (the server answers 404/400,
  // surfaced as 502), so the dangling-tool fallback must not render a control
  // for such a backend. Other backends keep the fallback for display only;
  // their confirm path still reports that no confirmation API exists.
  const pendingToolCall = computed<{ callId: string; name: string } | null>(() => {
    const session = options.activeSession.value
    if (session?.capabilities.toolConfirmation) {
      const approval = interactions.pendingApprovalFor(options.sessionId.value)
      return approval ? { callId: approval.requestId, name: approval.action } : null
    }
    const resolved = new Set<string>()
    let pending: { callId: string; name: string } | null = null
    for (const entry of options.activeMessages.value) {
      if (entry.kind === 'tool_result' && entry.callId) resolved.add(entry.callId)
      if (entry.kind === 'tool_call' && entry.callId) {
        pending = resolved.has(entry.callId) ? null : { callId: entry.callId, name: entry.name ?? entry.text }
      }
    }
    return pending
  })

  // Pending question forms. They travel on the same reply extension as tool
  // confirmation, so they are shown exactly when that capability is declared.
  // Rendered as answerable cards below the confirmation control: without them
  // an agent question blocks the turn forever with no way to answer.
  const pendingForms = computed(() => {
    const session = options.activeSession.value
    if (!session?.capabilities.toolConfirmation) return []
    return interactions.formsFor(options.sessionId.value)
  })

  // Approval polling. opencode pushes permission cards on its raw event stream;
  // a backend that declares tool confirmation without raw events (pi over RPC)
  // has no push channel, so the workbench polls while a turn runs and stops when
  // it settles. Replying already refreshes the list through the store.
  let poll: ReturnType<typeof setInterval> | null = null
  let pollBusy = false
  function stopApprovalPoll() {
    if (poll !== null) {
      clearInterval(poll)
      poll = null
    }
  }
  function startApprovalPoll() {
    stopApprovalPoll()
    const session = options.activeSession.value
    if (!session?.capabilities.toolConfirmation || session.capabilities.rawEvents) return
    poll = setInterval(() => {
      if (pollBusy) return
      pollBusy = true
      void interactions.loadApprovals(options.projectId.value, options.sessionId.value).finally(() => {
        pollBusy = false
      })
    }, 1500)
  }
  watch(options.isRunning, (running) => {
    if (running) startApprovalPoll()
    else stopApprovalPoll()
  })
  onBeforeUnmount(stopApprovalPoll)

  // Tool decisions for a backend that declares tool confirmation go through
  // its reply API (stores/interactions.ts); other backends still expose no
  // confirmation method, so the limitation is surfaced instead.
  async function decideTool(callId: string, decision: 'once' | 'reject') {
    const session = options.activeSession.value
    if (session?.capabilities.toolConfirmation) {
      toolNotice.value = ''
      const ok = await interactions.replyPermission(options.projectId.value, options.sessionId.value, callId, decision)
      if (!ok) toolNotice.value = interactions.error || '确认失败，请重试。'
      return
    }
    toolNotice.value = '此后端未提供工具确认接口，工作台无法批准或拒绝工具调用。'
  }

  function confirmTool(callId: string) {
    void decideTool(callId, 'once')
  }

  function denyTool(callId: string) {
    void decideTool(callId, 'reject')
  }

  const answeringForm = ref(false)

  // Submits one question-form answer. A settled/expired form drops its card
  // with the server's message instead of leaving an unanswerable question.
  async function answerForm(formId: string, answer: Record<string, unknown>) {
    if (answeringForm.value) return
    answeringForm.value = true
    toolNotice.value = ''
    try {
      const ok = await interactions.replyForm(options.projectId.value, options.sessionId.value, formId, answer)
      if (!ok) toolNotice.value = interactions.error || '提交失败，请重试。'
      else options.afterFormAnswer()
    } finally {
      answeringForm.value = false
    }
  }

  return {
    toolNotice,
    pendingToolCall,
    pendingForms,
    startApprovalPoll,
    confirmTool,
    denyTool,
    answerForm,
  }
}

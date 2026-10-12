<script setup lang="ts">
// Task conversation for the right pane of the shell. The task is an agent
// session of the project. Its stream is scoped to this session: opening another
// task closes the previous stream, so its events can never appear here. The
// composer is the only place a turn is sent or stopped; the send action becomes
// 停止 while the session's turn is running.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useAgentsStore } from '../stores/agents'
import { useSessionsStore, type ChatEntry } from '../stores/sessions'
import BackendTag from './BackendTag.vue'
import MessageView from './MessageView.vue'
import CapabilityControls from './CapabilityControls.vue'
import ModelSelect from './ModelSelect.vue'

const props = defineProps<{ projectId: number; sessionId: string }>()

const agents = useAgentsStore()
const sessions = useSessionsStore()
const router = useRouter()

const ready = ref(false)
const loadError = ref('')
const sendError = ref('')
const sending = ref(false)
const busy = ref(false)
// The delete confirmation is a modal inside the pane: the task leaves the
// platform index only after the user confirms.
const confirmingDelete = ref(false)
const deleting = ref(false)
const deleteError = ref('')
// preparing covers the window in which the task is being re-attached and its
// history replayed, so the pane says what it is waiting for instead of looking
// like an empty conversation.
const preparing = ref(false)
const messageText = ref('')
const selectedModel = ref('')
// Scrolling follows the newest entry only while the user is at the bottom.
// The log scrolls by hand in jsdom tests, so every read and write of the
// scroller geometry goes through the element itself.
const logScroll = ref<HTMLElement | null>(null)
const pinnedToBottom = ref(true)
const showJumpToLatest = ref(false)
// "At the bottom" tolerates a small rounding gap so subpixel layouts, and a
// wheel that stops a hair short, still count as following the conversation.
const FOLLOW_THRESHOLD_PX = 24
// Whether the model list for the open instance loaded. A failed list disables
// sending rather than falling back to a free-text model.
const modelOk = ref(true)
const toolNotice = ref('')

const activeSession = computed(() => sessions.sessionById(props.sessionId))
const instance = computed(() => {
  const session = activeSession.value
  if (!session) return null
  return agents.instances.find((entry) => entry.id === session.instanceId) ?? null
})

const activeMessages = computed<ChatEntry[]>(() => sessions.messagesFor(props.sessionId))
// A cheap change signal for the log: a new entry changes the length, and a
// streaming delta changes the text of an entry still open. Settled entries are
// immutable, so watching only the open ones catches every visible change
// without deep-watching the whole transcript.
const logSignature = computed(() => {
  const list = activeMessages.value
  let open = 0
  for (const entry of list) {
    if (entry.streaming) open += entry.text.length
  }
  return `${props.sessionId}:${list.length}:${open}`
})
const streamStatus = computed(() => sessions.statusFor(props.sessionId))
const historyError = computed(() => sessions.historyError[props.sessionId] ?? '')
const historyTruncated = computed(() => sessions.historyTruncated[props.sessionId] === true)

const modelSelection = computed(() => activeSession.value?.capabilities.modelSelection === true)
const isRunning = computed(() => activeSession.value?.turn === 'running')
const canInterrupt = computed(() => activeSession.value?.capabilities.interrupt === true)
// The composer is gated on the session itself. Opening a task repairs a
// stopped instance, so an instance's status is no longer a precondition the
// user has to fix from another pane.
const interactEnabled = computed(() => activeSession.value?.status === 'active')
// A session that is still not usable after opening says so, with the reason
// the backend gave, instead of leaving the user with a dead composer.
const unavailable = computed(
  () => activeSession.value !== null && activeSession.value.status === 'disconnected',
)

const composerLabel = computed(() => (isRunning.value && canInterrupt.value ? '停止' : '发送'))
const composerDisabled = computed(() => {
  if (!activeSession.value || !interactEnabled.value) return true
  if (isRunning.value) return !canInterrupt.value || busy.value
  return sending.value || messageText.value.trim() === '' || !modelOk.value
})

// The most recent tool call that has not produced a result yet. Only rendered
// by CapabilityControls when the session declares toolConfirmation.
const pendingToolCall = computed<{ callId: string; name: string } | null>(() => {
  const resolved = new Set<string>()
  let pending: { callId: string; name: string } | null = null
  for (const entry of activeMessages.value) {
    if (entry.kind === 'tool_result' && entry.callId) resolved.add(entry.callId)
    if (entry.kind === 'tool_call' && entry.callId) {
      pending = resolved.has(entry.callId) ? null : { callId: entry.callId, name: entry.name ?? entry.text }
    }
  }
  return pending
})

onMounted(open)
watch(() => [props.projectId, props.sessionId], open)
onBeforeUnmount(() => sessions.unsubscribeAll())

// onLogScroll tracks where the user is: reaching the bottom resumes following
// the conversation and retires the jump button; scrolling away stops following
// so a new entry can never yank the view.
function onLogScroll() {
  const el = logScroll.value
  if (!el) return
  const distance = el.scrollHeight - el.scrollTop - el.clientHeight
  const atBottom = distance <= FOLLOW_THRESHOLD_PX
  pinnedToBottom.value = atBottom
  if (atBottom) showJumpToLatest.value = false
}

function scrollToBottom(smooth: boolean) {
  const el = logScroll.value
  if (!el) return
  // The click animates; following the stream lands each delta immediately, so
  // a fast token stream never queues a pile of smooth scrolls.
  if (smooth && typeof el.scrollTo === 'function') {
    el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' })
  } else {
    el.scrollTop = el.scrollHeight
  }
  pinnedToBottom.value = true
  showJumpToLatest.value = false
}

watch(logSignature, () => {
  // Wait for the entry to be in the DOM before measuring it.
  void nextTick(() => {
    const el = logScroll.value
    if (!el) return
    if (pinnedToBottom.value) {
      el.scrollTop = el.scrollHeight
      showJumpToLatest.value = false
    } else {
      showJumpToLatest.value = true
    }
  })
})

// A directly opened task URL arrives before the project's sessions are loaded,
// so the list and the instances are fetched first. A failed load is reported as
// such; an id absent from a successful list is never opened. The model
// selection is reset per task, because it is chosen per message.
//
// Opening goes through the store's openSession, which re-attaches the session
// (starting a backend instance when none is running), replays the agent-native
// history, and only then subscribes — in that order.
async function open() {
  ready.value = false
  loadError.value = ''
  selectedModel.value = ''
  modelOk.value = true
  pinnedToBottom.value = true
  showJumpToLatest.value = false
  await Promise.all([sessions.loadSessions(props.projectId), agents.loadInstances(props.projectId)])
  ready.value = true
  if (sessions.error) {
    loadError.value = sessions.error
    return
  }
  if (!sessions.sessionById(props.sessionId)) return
  preparing.value = true
  try {
    await sessions.openSession(props.projectId, props.sessionId)
    // Opening a task lands on its newest entry, not on the top of a replayed
    // history.
    await nextTick()
    scrollToBottom(false)
  } finally {
    preparing.value = false
  }
}

// retryOpen re-runs the whole open path, which is what the unavailable note and
// the history retry need: both failures come from the same repair attempt.
function retryOpen() {
  void open()
}

// The frozen adapter contract exposes no method to answer a tool-permission
// request, so even a backend that advertised toolConfirmation could not be
// driven from here. The handlers surface the limitation instead.
function onToolDecision() {
  toolNotice.value = '此后端未提供工具确认接口，工作台无法批准或拒绝工具调用。'
}

async function send() {
  const session = activeSession.value
  if (!session || composerDisabled.value || messageText.value.trim() === '') return
  sending.value = true
  sendError.value = ''
  const model = modelSelection.value && selectedModel.value !== '' ? selectedModel.value : undefined
  const updated = await sessions.sendMessage(props.projectId, session.id, messageText.value, model)
  // The text is kept on failure so the user can retry without retyping it.
  if (updated) {
    messageText.value = ''
    // Sending means "look here": the view returns to the turn just made even
    // when it had been parked mid-history.
    await nextTick()
    scrollToBottom(false)
  } else sendError.value = sessions.error || '发送失败，请重试。'
  sending.value = false
}

async function stop() {
  const session = activeSession.value
  if (!session || !canInterrupt.value) return
  busy.value = true
  await sessions.interrupt(props.projectId, session.id)
  busy.value = false
}

async function onSubmit() {
  if (isRunning.value) {
    await stop()
    return
  }
  await send()
}

function reconnect() {
  if (activeSession.value) sessions.resubscribe(props.projectId, activeSession.value.id)
}

// removeTask deletes the platform index entry and returns to the project. The
// agent-native transcript is untouched, which the confirmation says.
async function removeTask() {
  const session = activeSession.value
  if (!session || deleting.value) return
  deleting.value = true
  deleteError.value = ''
  const removed = await sessions.deleteSession(props.projectId, session.id)
  deleting.value = false
  if (!removed) {
    deleteError.value = sessions.error || '删除失败，请重试。'
    return
  }
  confirmingDelete.value = false
  void router.push({ name: 'project', params: { projectId: String(props.projectId) } })
}
</script>

<template>
  <section
    class="task-chat"
    aria-label="任务对话"
    data-test="conversation"
    :data-stream-session="activeSession ? props.sessionId : ''"
    :data-stream-status="streamStatus"
  >
    <template v-if="activeSession">
      <header class="chat-header">
        <BackendTag :backend="activeSession.backend" />
        <h2 class="session-heading">{{ activeSession.title || activeSession.id }}</h2>
        <button
          type="button"
          class="delete-task"
          data-test="delete-task"
          title="删除任务"
          @click="confirmingDelete = true"
        >
          删除
        </button>
        <div class="session-meta">
          <span v-if="instance" class="chip">工作目录：{{ instance.directory }}</span>
          <span v-if="isRunning" class="turn-status" role="status" aria-live="polite">运行中</span>
        </div>
      </header>

      <p v-if="preparing" class="history-note" data-test="preparing" role="status" aria-live="polite">
        正在准备 agent…
      </p>

      <p v-if="historyTruncated" class="history-note" data-test="history-truncated">
        仅显示最近的历史记录。
      </p>

      <p v-if="historyError" class="history-error" role="alert" data-test="history-error">
        {{ historyError }}
        <button type="button" class="retry" data-test="history-retry" @click="retryOpen">重试</button>
      </p>

      <p v-if="unavailable" class="instance-note" role="alert" data-test="session-unavailable">
        {{ sessions.error || '任务当前不可用，请重试。' }}
        <button type="button" class="retry" data-test="session-retry" @click="retryOpen">重试</button>
      </p>

      <div class="conversation-area">
        <div
          class="conversation-scroll"
          data-test="conversation-scroll"
          ref="logScroll"
          @scroll.passive="onLogScroll"
        >
          <MessageView
            :messages="activeMessages"
            :capabilities="activeSession.capabilities"
            :stream-status="streamStatus"
          />

          <button v-if="streamStatus === 'error'" type="button" class="reconnect" @click="reconnect">
            重新连接事件流
          </button>
        </div>

        <button
          v-if="showJumpToLatest"
          type="button"
          class="jump-latest"
          data-test="jump-latest"
          @click="scrollToBottom(true)"
        >
          ↓ 滚动到最新消息
        </button>
      </div>

      <CapabilityControls
        :capabilities="activeSession.capabilities"
        :pending-tool-call="pendingToolCall"
        @confirm-tool="onToolDecision"
        @deny-tool="onToolDecision"
      />

      <p v-if="toolNotice" class="tool-notice" role="alert">{{ toolNotice }}</p>

      <p v-if="sendError" class="send-error" role="alert">{{ sendError }}</p>

      <form class="send" @submit.prevent="onSubmit">
        <label for="message-text">发送消息</label>
        <textarea
          id="message-text"
          v-model="messageText"
          rows="3"
          placeholder="向 Agent 发送消息…（Enter 发送，Shift+Enter 换行）"
          @keydown.enter.exact.prevent="onSubmit"
        ></textarea>
        <div class="composer-bar">
          <ModelSelect
            v-model="selectedModel"
            :project-id="props.projectId"
            :instance-id="activeSession.instanceId"
            :backend="activeSession.backend"
            :model-selection="activeSession.capabilities.modelSelection"
            @status="modelOk = $event"
          />
          <button
            type="button"
            class="composer-action"
            :class="isRunning ? 'stop-button' : 'send-button'"
            :disabled="composerDisabled"
            @click="onSubmit"
          >
            {{ composerLabel }}
          </button>
        </div>
      </form>

      <div
        v-if="confirmingDelete"
        class="confirm-backdrop"
        role="dialog"
        aria-modal="true"
        aria-label="删除任务"
        data-test="delete-task-dialog"
      >
        <div class="confirm-panel">
          <p class="confirm-title">确定删除任务「{{ activeSession.title || activeSession.id }}」吗？</p>
          <p class="confirm-hint">仅从平台列表移除；Agent 原生会话仍保留，重新扫描可能再次出现。</p>
          <p v-if="deleteError" class="confirm-error" role="alert">{{ deleteError }}</p>
          <div class="confirm-actions">
            <button type="button" class="confirm-cancel" @click="confirmingDelete = false">取消</button>
            <button
              type="button"
              class="confirm-delete"
              data-test="confirm-delete"
              :disabled="deleting"
              @click="removeTask"
            >
              删除
            </button>
          </div>
        </div>
      </div>
    </template>
    <template v-else-if="loadError">
      <p class="empty" role="alert">{{ loadError }}</p>
      <RouterLink :to="{ name: 'project', params: { projectId: String(props.projectId) } }">返回项目</RouterLink>
    </template>
    <template v-else-if="ready">
      <p class="empty">任务不存在或已被移除。</p>
      <RouterLink :to="{ name: 'project', params: { projectId: String(props.projectId) } }">返回项目</RouterLink>
    </template>
    <p v-else class="loading">加载中…</p>
  </section>
</template>

<style scoped>
.task-chat {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  /* The pane is one viewport tall, so the conversation scrolls inside it and
     the composer stays put at the bottom. */
  height: 100%;
  min-height: 0;
}

.conversation-area {
  /* The anchor for the floating jump button, which sits over the bottom of
     the log and above the composer. */
  position: relative;
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.conversation-scroll {
  flex: 1;
  min-height: 0;
  overflow: auto;
}

.jump-latest {
  position: absolute;
  bottom: 0.75rem;
  left: 50%;
  transform: translateX(-50%);
  z-index: 5;
  padding: 0.3rem 0.8rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 999px;
  background: var(--color-surface);
  box-shadow: 0 4px 12px rgba(15, 23, 42, 0.15);
  font: inherit;
  font-size: 0.85rem;
  cursor: pointer;
}

.chat-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.delete-task {
  padding: 0.15rem 0.5rem;
  border: 1px solid var(--color-danger-border);
  border-radius: 6px;
  background: var(--color-surface);
  color: var(--color-danger);
  font: inherit;
  font-size: 0.8rem;
  cursor: pointer;
}

.confirm-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.35);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 20;
}

.confirm-panel {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  width: min(26rem, 90vw);
  padding: 1rem 1.1rem;
  border-radius: 10px;
  background: var(--color-surface);
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.2);
}

.confirm-title {
  margin: 0;
  font-weight: 600;
}

.confirm-hint {
  margin: 0;
  color: var(--color-text-muted);
  font-size: 0.85rem;
}

.confirm-error {
  margin: 0;
  color: var(--color-danger);
  font-size: 0.85rem;
}

.confirm-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
}

.confirm-actions button {
  padding: 0.35rem 0.7rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  cursor: pointer;
  font: inherit;
}

.confirm-actions .confirm-delete {
  background: var(--color-danger);
  border-color: var(--color-danger);
  color: var(--color-surface);
}

.confirm-actions .confirm-delete:disabled {
  background: var(--color-danger-border);
  border-color: var(--color-danger-border);
  cursor: default;
}

.session-heading {
  margin: 0;
  font-size: 1.05rem;
}

.session-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.5rem;
}

.chip {
  color: var(--color-text-muted);
  background: var(--color-surface-muted);
  border-radius: 999px;
  padding: 0.1rem 0.5rem;
  font-size: 0.8rem;
}

.turn-status {
  color: var(--color-primary-hover);
  font-size: 0.85rem;
}

.history-note {
  margin: 0;
  color: var(--color-text-muted);
  font-size: 0.85rem;
}

.history-error {
  margin: 0;
  color: var(--color-warning);
  font-size: 0.85rem;
}

.retry {
  margin-left: 0.4rem;
  padding: 0.1rem 0.5rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  cursor: pointer;
  font: inherit;
}

.send {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.send textarea {
  padding: 0.4rem 0.6rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  font: inherit;
}

.composer-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.composer-action,
.reconnect {
  align-self: flex-start;
  padding: 0.35rem 0.7rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  cursor: pointer;
}

.composer-action.send-button {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: var(--color-surface);
}

.composer-action.send-button:disabled {
  background: var(--color-info-border);
  border-color: var(--color-info-border);
  cursor: default;
}

.composer-action.stop-button {
  color: var(--color-danger);
  border-color: var(--color-danger-border);
}

.tool-notice {
  color: var(--color-warning);
  font-size: 0.85rem;
  margin: 0;
}

.instance-note {
  color: var(--color-warning);
  font-size: 0.85rem;
  margin: 0;
}

.send-error {
  color: var(--color-danger);
  font-size: 0.85rem;
  margin: 0;
}

.empty,
.loading {
  color: var(--color-text-muted);
}
</style>

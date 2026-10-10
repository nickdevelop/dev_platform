<script setup lang="ts">
// Task conversation for the right pane of the shell. The task is an agent
// session of the project. Its stream is scoped to this session: opening another
// task closes the previous stream, so its events can never appear here. Only
// controls the session's declared capabilities support are rendered.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useSessionsStore, type ChatEntry } from '../stores/sessions'
import BackendTag from './BackendTag.vue'
import MessageView from './MessageView.vue'
import CapabilityControls from './CapabilityControls.vue'

const props = defineProps<{ projectId: number; sessionId: string }>()

const sessions = useSessionsStore()

const ready = ref(false)
const sessionModel = ref('')
const sending = ref(false)
const messageText = ref('')
const busy = ref(false)
const toolNotice = ref('')

const activeSession = computed(() => sessions.sessionById(props.sessionId))
const activeMessages = computed<ChatEntry[]>(() => sessions.messagesFor(props.sessionId))
const streamStatus = computed(() => sessions.statusFor(props.sessionId))

// Model input is rendered only when this session's own capabilities declare it;
// the session DTO is authoritative for what a message may carry.
const activeModelSelection = computed(() => activeSession.value?.capabilities.modelSelection === true)

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

// A directly opened task URL arrives before the project's sessions are loaded,
// so the list is fetched first. An id absent from the list is never subscribed.
async function open() {
  ready.value = false
  if (sessions.projectId !== props.projectId) await sessions.loadSessions(props.projectId)
  ready.value = true
  if (sessions.sessionById(props.sessionId)) {
    sessions.activateSession(props.projectId, props.sessionId)
  }
}

// The frozen adapter contract exposes no method to answer a tool-permission
// request, so even a backend that advertised toolConfirmation could not be
// driven from here. The handlers surface the limitation instead.
function onToolDecision() {
  toolNotice.value = '此后端未提供工具确认接口，工作台无法批准或拒绝工具调用。'
}

async function send() {
  const session = activeSession.value
  if (!session || sending.value || messageText.value.trim() === '') return
  sending.value = true
  const model =
    activeModelSelection.value && sessionModel.value.trim() !== ''
      ? sessionModel.value.trim()
      : undefined
  await sessions.sendMessage(props.projectId, session.id, messageText.value, model)
  messageText.value = ''
  sending.value = false
}

async function stop() {
  const session = activeSession.value
  if (!session) return
  busy.value = true
  await sessions.stopSession(props.projectId, session.id)
  busy.value = false
}

async function resume() {
  const session = activeSession.value
  if (!session) return
  busy.value = true
  await sessions.continueSession(props.projectId, session.id)
  busy.value = false
}

function reconnect() {
  if (activeSession.value) sessions.resubscribe(props.projectId, activeSession.value.id)
}

const statusLabels: Record<string, string> = {
  active: '进行中',
  stopped: '已停止',
  disconnected: '已断开',
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
        <h2 class="session-heading">{{ activeSession.title || activeSession.id }}</h2>
        <div class="session-meta">
          <BackendTag :backend="activeSession.backend" />
          <span class="session-status" role="status" aria-live="polite">
            状态：{{ statusLabels[activeSession.status] ?? activeSession.status }}
          </span>
        </div>
      </header>

      <p class="history-note" data-test="history-note">
        历史消息由 Agent 原生保存；工作台仅显示本次打开后的实时事件流，不会回放此前的会话记录。
      </p>

      <MessageView
        :messages="activeMessages"
        :capabilities="activeSession.capabilities"
        :stream-status="streamStatus"
      />

      <button v-if="streamStatus === 'error'" type="button" class="reconnect" @click="reconnect">
        重新连接事件流
      </button>

      <CapabilityControls
        :capabilities="activeSession.capabilities"
        :model="sessionModel"
        :pending-tool-call="pendingToolCall"
        :busy="busy"
        @update:model="sessionModel = $event"
        @stop="stop"
        @continue="resume"
        @confirm-tool="onToolDecision"
        @deny-tool="onToolDecision"
      />

      <p v-if="toolNotice" class="tool-notice" role="alert">{{ toolNotice }}</p>

      <form class="send" @submit.prevent="send">
        <label for="message-text">发送消息</label>
        <textarea id="message-text" v-model="messageText" rows="3"></textarea>
        <button
          type="submit"
          class="send-button"
          :disabled="sending || messageText.trim() === '' || activeSession.status !== 'active'"
        >
          发送
        </button>
      </form>
    </template>
    <p v-else-if="ready" class="empty">任务不存在或已被移除。</p>
    <p v-else class="loading">加载中…</p>
  </section>
</template>

<style scoped>
.task-chat {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  min-height: 0;
}

.chat-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
}

.session-heading {
  margin: 0;
  font-size: 1.05rem;
}

.session-meta {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.session-status {
  color: #475569;
  font-size: 0.85rem;
}

.history-note {
  margin: 0;
  color: #64748b;
  font-size: 0.85rem;
}

.send {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.send textarea {
  padding: 0.4rem 0.6rem;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  font: inherit;
}

.send button,
.reconnect {
  align-self: flex-start;
  padding: 0.35rem 0.7rem;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  background: #fff;
  cursor: pointer;
}

.send .send-button {
  background: #2563eb;
  border-color: #2563eb;
  color: #fff;
}

.send .send-button:disabled {
  background: #93c5fd;
  border-color: #93c5fd;
  cursor: default;
}

.tool-notice {
  color: #92400e;
  font-size: 0.85rem;
  margin: 0;
}

.empty,
.loading {
  color: #64748b;
}
</style>

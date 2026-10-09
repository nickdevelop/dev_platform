<script setup lang="ts">
// Unified session workbench: the project's session list beside the selected
// session's streamed message log, status and capability-gated controls. Only
// controls the session's declared capabilities support are rendered; the
// stream is scoped to the selected session so another instance's events can
// never appear here.
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useAgentsStore } from '../stores/agents'
import { useSessionsStore } from '../stores/sessions'
import type { ChatEntry } from '../stores/sessions'
import SessionList from '../components/SessionList.vue'
import MessageView from '../components/MessageView.vue'
import CapabilityControls from '../components/CapabilityControls.vue'

const route = useRoute()
const agents = useAgentsStore()
const sessions = useSessionsStore()

const projectId = computed(() => Number(route.params.id))
const selectedInstanceId = ref('')
const sessionTitle = ref('')
const sessionModel = ref('')
const creating = ref(false)
const sending = ref(false)
const messageText = ref('')
const busy = ref(false)

const activeSession = computed(() => sessions.activeSession)
const activeMessages = computed<ChatEntry[]>(() =>
  activeSession.value ? sessions.messagesFor(activeSession.value.id) : [],
)
const activeStreamStatus = computed(() =>
  activeSession.value ? sessions.statusFor(activeSession.value.id) : 'idle',
)

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

// Model selection is offered only when the selected instance's backend
// declares it. The backend re-checks the capability, so this is presentation.
const modelSelectionAvailable = computed(() => {
  const instance = agents.instances.find((entry) => entry.id === selectedInstanceId.value)
  if (!instance) return false
  return agents.capabilitiesFor(instance.backend)?.modelSelection === true
})

onMounted(load)
onBeforeUnmount(() => sessions.unsubscribeAll())

async function load() {
  await Promise.all([
    agents.loadBackends(),
    agents.loadInstances(projectId.value),
    sessions.loadSessions(projectId.value),
  ])
  const first = agents.runningInstances[0]
  if (selectedInstanceId.value === '' && first) selectedInstanceId.value = first.id
}

function selectSession(sessionId: string) {
  sessions.activateSession(projectId.value, sessionId)
}

async function createSession() {
  if (selectedInstanceId.value === '' || creating.value) return
  creating.value = true
  const body: { instanceId: string; title?: string; model?: string } = {
    instanceId: selectedInstanceId.value,
  }
  if (sessionTitle.value.trim() !== '') body.title = sessionTitle.value.trim()
  if (modelSelectionAvailable.value && sessionModel.value.trim() !== '') {
    body.model = sessionModel.value.trim()
  }
  const created = await sessions.createSession(projectId.value, body)
  if (created) {
    sessionTitle.value = ''
    sessions.activateSession(projectId.value, created.id)
  }
  creating.value = false
}

async function send() {
  const session = activeSession.value
  if (!session || sending.value || messageText.value.trim() === '') return
  sending.value = true
  const model =
    modelSelectionAvailable.value && sessionModel.value.trim() !== ''
      ? sessionModel.value.trim()
      : undefined
  await sessions.sendMessage(projectId.value, session.id, messageText.value, model)
  messageText.value = ''
  sending.value = false
}

async function stop() {
  const session = activeSession.value
  if (!session) return
  busy.value = true
  await sessions.stopSession(projectId.value, session.id)
  busy.value = false
}

async function resume() {
  const session = activeSession.value
  if (!session) return
  busy.value = true
  await sessions.continueSession(projectId.value, session.id)
  busy.value = false
}

function reconnect() {
  if (activeSession.value) sessions.resubscribe(projectId.value, activeSession.value.id)
}
</script>

<template>
  <main class="workbench">
    <header class="header">
      <h1>会话工作台</h1>
      <RouterLink class="back" :to="{ name: 'project-agents', params: { id: String(projectId) } }">
        返回实例设置
      </RouterLink>
    </header>

    <p v-if="sessions.error || agents.error" class="page-error" role="alert">
      {{ sessions.error || agents.error }}
    </p>

    <div class="layout">
      <section class="sessions" aria-label="会话列表">
        <form class="create-session" @submit.prevent="createSession">
          <label for="instance-select">在实例上新建会话</label>
          <select id="instance-select" v-model="selectedInstanceId">
            <option v-for="instance in agents.runningInstances" :key="instance.id" :value="instance.id">
              {{ instance.backend }} · {{ instance.id }}
            </option>
          </select>
          <input
            id="session-title"
            v-model="sessionTitle"
            type="text"
            placeholder="会话标题（可选）"
            autocomplete="off"
          />
          <input
            v-if="modelSelectionAvailable"
            id="new-session-model"
            v-model="sessionModel"
            type="text"
            placeholder="模型（可选）"
            autocomplete="off"
          />
          <button type="submit" class="create" :disabled="creating || selectedInstanceId === ''">
            新建会话
          </button>
        </form>

        <SessionList
          :sessions="sessions.sessions"
          :active-id="sessions.activeSessionId"
          @select="selectSession"
        />
      </section>

      <section class="conversation" aria-label="会话内容">
        <template v-if="activeSession">
          <h2 class="session-heading">{{ activeSession.title || activeSession.id }}</h2>
          <p class="session-status" role="status" aria-live="polite">
            状态：{{ activeSession.status === 'active' ? '进行中' : activeSession.status === 'stopped' ? '已停止' : '已断开' }}
          </p>

          <MessageView
            :messages="activeMessages"
            :capabilities="activeSession.capabilities"
            :stream-status="activeStreamStatus"
          />

          <button
            v-if="activeStreamStatus === 'error'"
            type="button"
            class="reconnect"
            @click="reconnect"
          >
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
          />

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
        <p v-else class="empty">从左侧选择一个会话。</p>
      </section>
    </div>
  </main>
</template>

<style scoped>
.workbench {
  max-width: 70rem;
  margin: 0 auto;
  padding: 1rem;
}

.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
}

.header h1 {
  font-size: 1.25rem;
  margin: 0;
}

.layout {
  display: grid;
  grid-template-columns: minmax(16rem, 22rem) 1fr;
  gap: 1rem;
  margin-top: 1rem;
}

.create-session,
.send {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  margin-bottom: 0.75rem;
}

.create-session select,
.create-session input,
.send textarea {
  padding: 0.4rem 0.6rem;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  font: inherit;
}

.create-session button,
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

.session-heading {
  margin: 0 0 0.25rem;
  font-size: 1.05rem;
}

.session-status {
  margin: 0 0 0.5rem;
  color: #475569;
}

.page-error {
  color: #b91c1c;
}

.empty {
  color: #64748b;
}
</style>

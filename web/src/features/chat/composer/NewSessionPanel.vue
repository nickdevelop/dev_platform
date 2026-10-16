<script setup lang="ts">
// New session form for the right pane of the shell, laid out like the opencode
// new-session view: a centered composer holds the first message, and the
// toolbar inside it carries the agent-backend switch, the model picker and
// the create action. A session names only an agent backend: the server reuses
// or starts the project's instance for that backend. The title is never
// entered here — the server derives it from the first message — so an
// optional message rides along with the create request and the waiting is
// visible on this form instead of a frozen composer. On success the shell
// navigates to the new session.
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useAgentsStore } from '../../../stores/agents'
import { useSessionsStore } from '../../../stores/sessions'
import type { BackendId } from '../../../api/agents'
import ModelSelect from './ModelSelect.vue'

const props = defineProps<{ projectId: number }>()

const emit = defineEmits<{ created: [sessionId: string] }>()

const agents = useAgentsStore()
const sessions = useSessionsStore()

const selectedBackend = ref<BackendId | ''>('')
const messageText = ref('')
const selectedModel = ref('')
const modelOk = ref(false)
const creating = ref(false)
const composerBox = ref<HTMLElement | null>(null)

const selectedCapabilities = computed(() =>
  selectedBackend.value === '' ? null : agents.capabilitiesFor(selectedBackend.value),
)

const selectedEnabled = computed(
  () => selectedBackend.value !== '' && agents.backendEnabledFor(selectedBackend.value),
)

// A first message that needs a model (the backend supports choosing one) waits
// for the model list, which is always available: a backend's catalog is scoped to
// the project, not to a running instance, so creating the very first session of a
// project can still name a model. Until the list arrives the create is held back
// rather than silently sending on the backend's default model.
const firstMessageReady = computed(() => {
  if (messageText.value.trim() === '') return true
  if (!selectedCapabilities.value?.modelSelection) return true
  return modelOk.value && selectedModel.value !== ''
})

// Only platform-enabled backends may be chosen for a new session. The server also
// enforces this, but hiding them keeps the UI honest.
const selectableBackends = computed(() => agents.backends.filter((entry) => entry.enabled !== false))

const canCreate = computed(
  () => selectedBackend.value !== '' && selectedEnabled.value && firstMessageReady.value && !creating.value,
)

const createLabel = computed(() => {
  if (creating.value) return '创建中…'
  return messageText.value.trim() !== '' ? '创建并开始对话' : '新建会话'
})

onMounted(async () => {
  await agents.loadBackends()
  if (selectedBackend.value === '' && selectableBackends.value.length > 0) {
    selectedBackend.value = selectableBackends.value[0].backend
  }
  composerBox.value?.querySelector('textarea')?.focus()
})

function selectBackend(backend: BackendId) {
  if (!agents.backendEnabledFor(backend)) return
  if (selectedBackend.value === backend) return
  selectedBackend.value = backend
  selectedModel.value = ''
  modelOk.value = false
}

async function createSession() {
  if (selectedBackend.value === '' || creating.value || !selectedEnabled.value) return
  if (!firstMessageReady.value) return
  creating.value = true
  const text = messageText.value.trim()
  // The model is part of the session's identity, not only of its first turn, so
  // it rides the create request even when no message is sent: the session is
  // created on the model the user saw, and the conversation opens on it.
  const withModel =
    selectedCapabilities.value?.modelSelection === true &&
    modelOk.value &&
    selectedModel.value !== ''
  const created = await sessions.createSession(props.projectId, {
    backend: selectedBackend.value,
    ...(text !== '' ? { message: text } : {}),
    ...(withModel ? { model: selectedModel.value } : {}),
  })
  creating.value = false
  if (created) emit('created', created.id)
}

function onComposerEnter() {
  void createSession()
}
</script>

<template>
  <section class="new-session" aria-label="新建会话">
    <div class="new-session-inner">
      <h1>新建会话</h1>
      <p class="subtitle">选择 Agent 后端，描述你要做的事</p>

      <form class="create-session" @submit.prevent="createSession">
        <div class="backend-switch" role="radiogroup" aria-label="Agent 后端">
          <button
            v-for="backend in selectableBackends"
            :key="backend.backend"
            type="button"
            role="radio"
            class="backend-option"
            :class="{ active: selectedBackend === backend.backend }"
            :aria-checked="selectedBackend === backend.backend"
            :data-backend="backend.backend"
            :title="backend.label"
            @click="selectBackend(backend.backend)"
          >
            <span class="backend-name">{{ backend.label || backend.backend }}</span>
          </button>
        </div>
        <p v-if="selectedCapabilities" class="hint">
          {{
            selectedCapabilities.modelSelection
              ? '支持模型选择；可在下方输入框内选定模型，之后也可在对话页更换。'
              : '不支持模型选择；模型将使用后端默认值。'
          }}
        </p>

        <div ref="composerBox" class="composer-box">
          <label class="composer-label" for="first-message">会话描述</label>
          <textarea
            id="first-message"
            v-model="messageText"
            rows="5"
            placeholder="描述会话，例如：帮我检查构建失败的原因…"
            @keydown.enter.exact.prevent="onComposerEnter"
          ></textarea>
          <div class="composer-bar">
            <div class="composer-tools">
              <ModelSelect
                v-if="selectedCapabilities?.modelSelection && selectedBackend !== ''"
                v-model="selectedModel"
                :key="selectedBackend"
                :project-id="props.projectId"
                :backend="selectedBackend"
                :model-selection="true"
                @status="modelOk = $event"
              />
              <span class="composer-hint">Enter 创建 · Shift+Enter 换行</span>
            </div>
            <button type="submit" class="create send-button" :disabled="!canCreate">
              {{ createLabel }}
            </button>
          </div>
        </div>
        <p v-if="messageText.trim() !== '' && !firstMessageReady" class="hint" data-test="model-required-hint">
          模型列表尚未就绪，暂时无法发送首条消息。
        </p>

        <!-- A refused session usually means the machine-wide service is not running.
             The platform will not start it for you, so the refusal says where to. -->
        <p v-if="sessions.error" class="page-error" role="alert" data-test="new-session-error">
          {{ sessions.error }}
          <RouterLink
            v-if="sessions.errorCode === 'service_not_running'"
            :to="{ name: 'settings-agents' }"
            data-test="new-session-service-link"
            >前往平台设置 → Agent</RouterLink
          >
        </p>
      </form>
    </div>
  </section>
</template>

<style scoped>
.new-session {
  display: flex;
  justify-content: center;
  padding: 3rem 1rem;
}

.new-session-inner {
  width: min(48rem, 100%);
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.new-session h1 {
  font-size: 1.5rem;
  margin: 0;
  text-align: center;
}

.subtitle {
  margin: 0 0 0.75rem;
  text-align: center;
  color: var(--color-text-muted);
  font-size: 0.9rem;
}

.create-session {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}

/* Segmented switch for the agent backend: one pill container, one option per
   backend, the active one raised. A project-disabled backend stays visible
   but unpressable so the user knows why it cannot be chosen. */
.backend-switch {
  display: flex;
  gap: 0.25rem;
  padding: 0.25rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 999px;
  background: var(--color-surface-muted);
  overflow-x: auto;
}

.backend-option {
  flex: 1 1 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.4rem;
  white-space: nowrap;
  padding: 0.4rem 0.9rem;
  border: 1px solid transparent;
  border-radius: 999px;
  background: transparent;
  color: var(--color-text-muted);
  font: inherit;
  font-size: 0.9rem;
  cursor: pointer;
}

.backend-option:hover:not(:disabled):not(.active) {
  color: var(--color-text);
}

.backend-option.active {
  background: var(--color-surface);
  border-color: var(--color-border-strong);
  color: var(--color-text);
  font-weight: 600;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.12);
}

.backend-option:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

/* Desktop-style composer: one rounded box holds the input and its toolbar,
   like the opencode new-session view and the conversation composer. */
.composer-box {
  border: 1px solid var(--color-border-strong);
  border-radius: 12px;
  background: var(--color-surface);
  padding: 0.6rem 0.7rem 0.5rem;
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.06);
}

.composer-box:focus-within {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 3px var(--color-info-soft);
}

.composer-label {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
}

.composer-box textarea {
  padding: 0.15rem 0.1rem;
  border: none;
  outline: none;
  resize: vertical;
  font: inherit;
  line-height: 1.5;
  background: transparent;
  color: var(--color-text);
  min-height: 6rem;
  max-height: 16rem;
}

.composer-box textarea::placeholder {
  color: var(--color-text-muted);
}

.composer-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding-top: 0.35rem;
  border-top: 1px solid var(--color-border);
}

.composer-tools {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  min-width: 0;
  flex-wrap: wrap;
}

.composer-hint {
  color: var(--color-text-muted);
  font-size: 0.75rem;
  white-space: nowrap;
}

.create.send-button {
  min-width: 7rem;
  padding: 0.4rem 1rem;
  border: 1px solid var(--color-primary);
  border-radius: 999px;
  background: var(--color-primary);
  color: var(--color-surface);
  font: inherit;
  font-weight: 600;
  cursor: pointer;
}

.create.send-button:disabled {
  background: var(--color-info-border);
  border-color: var(--color-info-border);
  cursor: default;
}

.hint {
  color: var(--color-text-muted);
  font-size: 0.85rem;
  margin: 0;
}

.page-error {
  color: var(--color-danger);
  font-size: 0.85rem;
  margin: 0;
}
</style>

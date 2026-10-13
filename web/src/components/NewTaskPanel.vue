<script setup lang="ts">
// New task form for the right pane of the shell. A task names only an agent
// backend: the server reuses or starts the project's instance for that backend.
// An optional first message and model ride along: the session is created on
// that model and the message is admitted as its first turn in one request, so
// the work the old two-step flow did in two serial calls happens once, with
// the waiting visible on this form instead of a frozen composer. The model is
// still chosen in the chat composer afterwards; native-session discovery lives
// in project settings. On success the shell navigates to the new task.
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useAgentsStore } from '../stores/agents'
import { useSessionsStore } from '../stores/sessions'
import type { BackendId } from '../api/agents'
import BackendTag from './BackendTag.vue'
import ModelSelect from './ModelSelect.vue'

const props = defineProps<{ projectId: number }>()

const emit = defineEmits<{ created: [sessionId: string] }>()

const agents = useAgentsStore()
const sessions = useSessionsStore()

const selectedBackend = ref<BackendId | ''>('')
const sessionTitle = ref('')
const messageText = ref('')
const selectedModel = ref('')
const modelOk = ref(false)
const creating = ref(false)

const selectedCapabilities = computed(() =>
  selectedBackend.value === '' ? null : agents.capabilitiesFor(selectedBackend.value),
)

const selectedEnabled = computed(
  () => selectedBackend.value !== '' && agents.backendEnabledFor(selectedBackend.value),
)

// candidateInstance is the running instance the model picker can load from:
// the create request will reuse the project's running instance of the backend,
// and that instance already knows its model list. Without one the list cannot
// load and a first message cannot pick a model.
const candidateInstance = computed(() => {
  if (selectedBackend.value === '') return null
  const running = agents.runningInstances.filter((entry) => entry.backend === selectedBackend.value)
  return running.length > 0 ? running[running.length - 1] : null
})

// A first message that needs a model (the backend supports choosing one) can
// only be sent when the picker actually loaded models. Without a message — or
// on a backend without model selection — the backend default applies and the
// create is always ready.
const firstMessageReady = computed(() => {
  if (messageText.value.trim() === '') return true
  if (!selectedCapabilities.value?.modelSelection) return true
  if (!candidateInstance.value) return false
  return modelOk.value && selectedModel.value !== ''
})

// Only platform-enabled backends may be chosen for a new task. The server also
// enforces this, but hiding them keeps the UI honest.
const selectableBackends = computed(() => agents.backends.filter((entry) => entry.enabled !== false))

onMounted(async () => {
  await Promise.all([
    agents.loadBackends(),
    agents.loadBackendConfigs(props.projectId),
    agents.loadInstances(props.projectId),
  ])
  if (selectedBackend.value === '' && selectableBackends.value.length > 0) {
    selectedBackend.value = selectableBackends.value[0].backend
  }
})

async function createSession() {
  if (selectedBackend.value === '' || creating.value || !selectedEnabled.value) return
  if (!firstMessageReady.value) return
  creating.value = true
  const text = messageText.value.trim()
  const withModel =
    text !== '' &&
    selectedCapabilities.value?.modelSelection === true &&
    modelOk.value &&
    selectedModel.value !== ''
  const created = await sessions.createSession(props.projectId, {
    backend: selectedBackend.value,
    title: sessionTitle.value.trim() === '' ? undefined : sessionTitle.value.trim(),
    ...(text !== '' ? { message: text } : {}),
    ...(withModel ? { model: selectedModel.value } : {}),
  })
  creating.value = false
  if (created) emit('created', created.id)
}
</script>

<template>
  <section class="new-task" aria-label="新建任务">
    <h1>新建任务</h1>

    <form class="create-session" @submit.prevent="createSession">
      <label for="backend-select">Agent 后端</label>
      <div class="backend-row">
        <select id="backend-select" v-model="selectedBackend">
          <option
            v-for="backend in selectableBackends"
            :key="backend.backend"
            :value="backend.backend"
            :disabled="!agents.backendEnabledFor(backend.backend)"
          >
            {{ backend.backend }}{{ agents.backendEnabledFor(backend.backend) ? '' : '（已在项目中禁用）' }}
          </option>
        </select>
        <BackendTag v-if="selectedBackend !== ''" :backend="selectedBackend" />
      </div>
      <p v-if="selectedCapabilities" class="hint">
        {{
          selectedCapabilities.modelSelection
            ? '支持模型选择与工具确认；可在下方随首条消息选定模型，之后也可在对话页更换。'
            : '不支持模型选择；模型将使用后端默认值。'
        }}
      </p>

      <label for="session-title">任务标题</label>
      <input
        id="session-title"
        v-model="sessionTitle"
        type="text"
        placeholder="标题（可选，留空时取首条消息）"
        autocomplete="off"
      />

      <label for="first-message">第一条消息</label>
      <textarea
        id="first-message"
        v-model="messageText"
        rows="3"
        placeholder="向 Agent 发出的第一条消息（可选，填写后创建即开始对话）"
      ></textarea>

      <template v-if="selectedCapabilities?.modelSelection">
        <label for="new-task-model">模型</label>
        <ModelSelect
          v-if="candidateInstance"
          v-model="selectedModel"
          :project-id="props.projectId"
          :instance-id="candidateInstance.id"
          :backend="selectedBackend"
          :model-selection="true"
          @status="modelOk = $event"
        />
        <p v-else class="hint" data-test="model-unavailable">
          该后端当前没有运行中的实例，模型列表不可用；首条消息暂不能随任务一起发送，可先创建任务，在对话页发送。
        </p>
      </template>
      <p v-if="messageText.trim() !== '' && !firstMessageReady" class="hint" data-test="model-required-hint">
        模型列表尚未就绪，暂时无法发送首条消息。
      </p>

      <!-- A refused task usually means the machine-wide service is not running.
           The platform will not start it for you, so the refusal says where to. -->
      <p v-if="sessions.error" class="page-error" role="alert" data-test="new-task-error">
        {{ sessions.error }}
        <RouterLink
          v-if="sessions.errorCode === 'service_not_running'"
          :to="{ name: 'settings-agents' }"
          data-test="new-task-service-link"
          >前往平台设置 → Agent</RouterLink
        >
      </p>

      <div class="actions">
        <button
          type="submit"
          class="create"
          :disabled="creating || selectedBackend === '' || !selectedEnabled || !firstMessageReady"
        >
          {{ creating ? '创建中…' : messageText.trim() !== '' ? '创建并开始对话' : '新建任务' }}
        </button>
      </div>
    </form>
  </section>
</template>

<style scoped>
.new-task {
  max-width: 36rem;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.new-task h1 {
  font-size: 1.25rem;
  margin: 0 0 0.5rem;
}

.create-session {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.backend-row,
.actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.create-session select,
.create-session input,
.create-session textarea {
  padding: 0.4rem 0.6rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  font: inherit;
}

.create-session textarea {
  resize: vertical;
}

.create-session button {
  padding: 0.35rem 0.7rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  cursor: pointer;
}

.create-session .create {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: var(--color-surface);
}

.create-session button:disabled {
  opacity: 0.5;
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

<script setup lang="ts">
// New task form for the right pane of the shell. A task names only an agent
// backend: the server reuses or starts the project's instance for that backend.
// The model is chosen in the chat composer, and native-session discovery lives
// in project settings. On success the shell navigates to the new task.
import { computed, onMounted, ref } from 'vue'
import { useAgentsStore } from '../stores/agents'
import { useSessionsStore } from '../stores/sessions'
import type { BackendId } from '../api/agents'
import BackendTag from './BackendTag.vue'

const props = defineProps<{ projectId: number }>()

const emit = defineEmits<{ created: [sessionId: string] }>()

const agents = useAgentsStore()
const sessions = useSessionsStore()

const selectedBackend = ref<BackendId | ''>('')
const sessionTitle = ref('')
const creating = ref(false)

const selectedCapabilities = computed(() =>
  selectedBackend.value === '' ? null : agents.capabilitiesFor(selectedBackend.value),
)

const selectedEnabled = computed(
  () => selectedBackend.value !== '' && agents.backendEnabledFor(selectedBackend.value),
)

onMounted(async () => {
  await Promise.all([agents.loadBackends(), agents.loadBackendConfigs(props.projectId)])
  if (selectedBackend.value === '' && agents.backends.length > 0) {
    selectedBackend.value = agents.backends[0].backend
  }
})

async function createSession() {
  if (selectedBackend.value === '' || creating.value || !selectedEnabled.value) return
  creating.value = true
  const created = await sessions.createSession(props.projectId, {
    backend: selectedBackend.value,
    title: sessionTitle.value.trim() === '' ? undefined : sessionTitle.value.trim(),
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
            v-for="backend in agents.backends"
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
            ? '支持模型选择与工具确认；模型在对话页选择。'
            : '不支持模型选择；模型将使用后端默认值。'
        }}
      </p>

      <label for="session-title">任务标题</label>
      <input
        id="session-title"
        v-model="sessionTitle"
        type="text"
        placeholder="标题（可选）"
        autocomplete="off"
      />

      <p v-if="sessions.error" class="page-error" role="alert">{{ sessions.error }}</p>

      <div class="actions">
        <button
          type="submit"
          class="create"
          :disabled="creating || selectedBackend === '' || !selectedEnabled"
        >
          新建任务
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
.create-session input {
  padding: 0.4rem 0.6rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  font: inherit;
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

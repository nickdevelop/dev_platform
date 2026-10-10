<script setup lang="ts">
// New task form for the right pane of the shell. A task is created as a session
// on a running instance; model input and native-session discovery are offered
// only when the selected instance's backend declares them. On success the shell
// navigates to the new task.
import { computed, onMounted, ref } from 'vue'
import { useAgentsStore } from '../stores/agents'
import { useSessionsStore } from '../stores/sessions'
import type { CreateSessionBody } from '../api/sessions'
import BackendTag from './BackendTag.vue'

const props = defineProps<{ projectId: number }>()

const emit = defineEmits<{ created: [sessionId: string] }>()

const agents = useAgentsStore()
const sessions = useSessionsStore()

const selectedInstanceId = ref('')
const sessionTitle = ref('')
const sessionModel = ref('')
const creating = ref(false)
const discovering = ref(false)

const selectedInstance = computed(() =>
  agents.runningInstances.find((entry) => entry.id === selectedInstanceId.value) ?? null,
)

const modelSelectionAvailable = computed(
  () => selectedInstance.value !== null && agents.capabilitiesFor(selectedInstance.value.backend)?.modelSelection === true,
)

const discoverAvailable = computed(
  () => selectedInstance.value !== null && agents.capabilitiesFor(selectedInstance.value.backend)?.sessionDiscovery === true,
)

onMounted(async () => {
  await Promise.all([agents.loadBackends(), agents.loadInstances(props.projectId)])
  if (selectedInstanceId.value === '' && agents.runningInstances.length > 0) {
    selectedInstanceId.value = agents.runningInstances[0].id
  }
})

async function createSession() {
  if (selectedInstanceId.value === '' || creating.value) return
  creating.value = true
  const body: CreateSessionBody = { instanceId: selectedInstanceId.value }
  if (sessionTitle.value.trim() !== '') body.title = sessionTitle.value.trim()
  if (modelSelectionAvailable.value && sessionModel.value.trim() !== '') {
    body.model = sessionModel.value.trim()
  }
  const created = await sessions.createSession(props.projectId, body)
  creating.value = false
  if (created) emit('created', created.id)
}

async function discover() {
  if (selectedInstanceId.value === '' || discovering.value) return
  discovering.value = true
  await sessions.discoverNativeSessions(props.projectId, selectedInstanceId.value)
  discovering.value = false
}
</script>

<template>
  <section class="new-task" aria-label="新建任务">
    <h1>新建任务</h1>

    <form class="create-session" @submit.prevent="createSession">
      <label for="instance-select">运行实例</label>
      <div class="instance-row">
        <select id="instance-select" v-model="selectedInstanceId">
          <option v-for="instance in agents.runningInstances" :key="instance.id" :value="instance.id">
            {{ instance.backend }} · {{ instance.id }}
          </option>
        </select>
        <BackendTag v-if="selectedInstance" :backend="selectedInstance.backend" />
      </div>
      <p v-if="agents.runningInstances.length === 0" class="hint">请先在项目设置中启动实例。</p>

      <label for="session-title">任务标题</label>
      <input id="session-title" v-model="sessionTitle" type="text" placeholder="标题（可选）" autocomplete="off" />

      <template v-if="modelSelectionAvailable">
        <label for="new-session-model">模型</label>
        <input id="new-session-model" v-model="sessionModel" type="text" placeholder="模型（可选）" autocomplete="off" />
      </template>

      <div class="actions">
        <button type="submit" class="create" :disabled="creating || selectedInstanceId === ''">
          新建任务
        </button>
        <button
          v-if="discoverAvailable"
          type="button"
          class="discover"
          data-test="discover-sessions"
          :disabled="discovering || selectedInstanceId === ''"
          @click="discover"
        >
          发现原生会话
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

.instance-row,
.actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.create-session select,
.create-session input {
  padding: 0.4rem 0.6rem;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  font: inherit;
}

.create-session button {
  padding: 0.35rem 0.7rem;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  background: #fff;
  cursor: pointer;
}

.create-session .create {
  background: #2563eb;
  border-color: #2563eb;
  color: #fff;
}

.create-session button:disabled {
  opacity: 0.5;
  cursor: default;
}

.hint {
  color: #64748b;
  font-size: 0.85rem;
  margin: 0;
}
</style>

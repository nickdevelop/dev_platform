<script setup lang="ts">
// Project Agent settings: lists the project's agent instances and lets the
// operator start another instance of a registered backend or stop a running
// one. Starting or stopping an instance never affects the project's other
// instances.
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAgentsStore } from '../stores/agents'
import type { BackendId } from '../api/agents'
import AgentInstanceList from '../components/AgentInstanceList.vue'
import BackendDiagnostics from '../components/BackendDiagnostics.vue'

const route = useRoute()
const router = useRouter()
const agents = useAgentsStore()

const projectId = computed(() => Number(route.params.id))
const selectedBackend = ref<BackendId | ''>('')
const starting = ref(false)
const actionError = ref('')

onMounted(load)

async function load() {
  await Promise.all([
    agents.loadBackends(),
    agents.loadInstances(projectId.value),
    agents.loadDiagnostics(),
    agents.loadBackendConfigs(projectId.value),
  ])
  if (selectedBackend.value === '' && agents.backends.length > 0) {
    selectedBackend.value = agents.backends[0].backend
  }
}

async function start() {
  if (selectedBackend.value === '' || starting.value) return
  starting.value = true
  actionError.value = ''
  const created = await agents.startInstance(projectId.value, selectedBackend.value)
  if (!created) actionError.value = agents.error
  starting.value = false
}

async function stop(instanceId: string) {
  actionError.value = ''
  const stopped = await agents.stopInstance(projectId.value, instanceId)
  if (!stopped) actionError.value = agents.error
}

async function toggleBackend(backend: BackendId, event: Event) {
  const enabled = (event.target as HTMLInputElement).checked
  actionError.value = ''
  const updated = await agents.setBackendEnabled(projectId.value, backend, enabled)
  if (!updated) actionError.value = agents.error
}

// The start control is disabled when the selected backend is disabled for the
// project, so the UI never offers an action the server will refuse.
const selectedBackendEnabled = computed(
  () => selectedBackend.value === '' || agents.backendEnabledFor(selectedBackend.value),
)

function openWorkbench() {
  void router.push({ name: 'session-workbench', params: { id: String(projectId.value) } })
}
</script>

<template>
  <main class="project-agents">
    <header class="header">
      <h1>Agent 实例设置</h1>
      <button type="button" class="open-workbench" @click="openWorkbench">打开会话工作台</button>
    </header>

    <p v-if="agents.error || actionError" class="page-error" role="alert">
      {{ actionError || agents.error }}
    </p>
    <p v-else-if="agents.diagnosticsError" class="diagnostics-error" role="alert">
      {{ agents.diagnosticsError }}
    </p>

    <BackendDiagnostics :diagnostics="agents.diagnostics" />

    <section class="backend-config" aria-label="项目 Agent 后端启用">
      <h2>项目后端启用</h2>
      <ul class="backend-config-list">
        <li v-for="backend in agents.backends" :key="backend.backend" class="backend-config-item">
          <label>
            <input
              type="checkbox"
              :checked="agents.backendEnabledFor(backend.backend)"
              :data-test="`backend-toggle-${backend.backend}`"
              @change="toggleBackend(backend.backend, $event)"
            />
            启用 {{ backend.backend }}
          </label>
        </li>
      </ul>
      <p class="hint">未显式禁用的后端默认启用；禁用的后端无法在该项目中启动实例。</p>
    </section>

    <section class="start-instance" aria-label="启动 Agent 实例">
      <label for="backend-select">Agent 后端</label>
      <select id="backend-select" v-model="selectedBackend">
        <option v-for="backend in agents.backends" :key="backend.backend" :value="backend.backend">
          {{ backend.backend }}
        </option>
      </select>
      <button
        type="button"
        class="start"
        :disabled="starting || selectedBackend === '' || !selectedBackendEnabled"
        @click="start"
      >
        启动实例
      </button>
      <p class="hint">每个实例相互独立：一个实例的失败或停止不影响其他实例。</p>
    </section>

    <p v-if="agents.loading" class="loading">加载中…</p>
    <AgentInstanceList
      v-else
      :instances="agents.instances"
      :capabilities-for="agents.capabilitiesFor"
      @stop="stop"
    />
  </main>
</template>

<style scoped>
.project-agents {
  max-width: 60rem;
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

.start-instance {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
  margin: 1rem 0;
}

.start-instance select,
.start-instance button,
.open-workbench {
  padding: 0.35rem 0.7rem;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  background: #fff;
  cursor: pointer;
}

.start-instance .hint {
  flex-basis: 100%;
  color: #64748b;
  font-size: 0.85rem;
  margin: 0;
}

.page-error {
  color: #b91c1c;
}

.diagnostics-error {
  color: #92400e;
}
</style>

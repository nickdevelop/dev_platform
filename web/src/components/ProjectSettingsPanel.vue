<script setup lang="ts">
// Project settings for the right pane of the shell: project facts, the project's
// Agent backends and running instances, and the danger-zone actions. The actions
// are handed up as `manage` events so the shell owns the single manage dialog.
// Starting or stopping an instance never affects the project's other instances.
import { computed, onMounted, ref, watch } from 'vue'
import type { BackendId } from '../api/agents'
import type { Project } from '../api/projects'
import { useAgentsStore } from '../stores/agents'
import { useProjectsStore } from '../stores/projects'
import AgentInstanceList from './AgentInstanceList.vue'
import BackendDiagnostics from './BackendDiagnostics.vue'

const props = defineProps<{ projectId: number }>()

const emit = defineEmits<{
  manage: [kind: 'move' | 'rename' | 'unregister' | 'delete', project: Project]
}>()

const projectsStore = useProjectsStore()
const agents = useAgentsStore()

const project = computed(() => projectsStore.byId(props.projectId) ?? null)
const selectedBackend = ref<BackendId | ''>('')
const starting = ref(false)
const actionError = ref('')

onMounted(load)
watch(() => props.projectId, load)

async function load() {
  await agents.loadBackends()
  if (selectedBackend.value === '' && agents.backends.length > 0) {
    selectedBackend.value = agents.backends[0].backend
  }
  await Promise.all([
    agents.loadInstances(props.projectId),
    agents.loadDiagnostics(),
    agents.loadBackendConfigs(props.projectId),
  ])
}

async function start() {
  if (selectedBackend.value === '' || starting.value) return
  starting.value = true
  actionError.value = ''
  const created = await agents.startInstance(props.projectId, selectedBackend.value)
  if (!created) actionError.value = agents.error
  starting.value = false
}

async function stop(instanceId: string) {
  actionError.value = ''
  const stopped = await agents.stopInstance(props.projectId, instanceId)
  if (!stopped) actionError.value = agents.error
}

async function toggleBackend(backend: BackendId, event: Event) {
  const enabled = (event.target as HTMLInputElement).checked
  actionError.value = ''
  const updated = await agents.setBackendEnabled(props.projectId, backend, enabled)
  if (!updated) actionError.value = agents.error
}

// The start control is disabled when the selected backend is disabled for the
// project, so the UI never offers an action the server will refuse.
const selectedBackendEnabled = computed(
  () => selectedBackend.value === '' || agents.backendEnabledFor(selectedBackend.value),
)

function manage(kind: 'move' | 'rename' | 'unregister' | 'delete') {
  if (project.value) emit('manage', kind, project.value)
}
</script>

<template>
  <section class="project-settings" aria-label="项目设置">
    <header class="header">
      <div>
        <h1>{{ project?.name ?? '项目不可用' }}</h1>
        <p class="path">{{ project?.path }}</p>
      </div>
      <div class="actions">
        <button type="button" @click="manage('rename')">重命名</button>
        <button type="button" @click="manage('move')">移动</button>
        <button type="button" class="unregister" @click="manage('unregister')">解除登记</button>
        <button type="button" class="delete" @click="manage('delete')">删除目录</button>
      </div>
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

    <section class="resources" aria-label="资源关联">
      <h2>资源关联</h2>
      <p class="hint">待资源管理功能接入。Skill、MCP 与提示词将在资源管理功能完成后显示在此处。</p>
    </section>
  </section>
</template>

<style scoped>
.project-settings {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
}

.header h1 {
  font-size: 1.25rem;
  margin: 0;
}

.path {
  color: #64748b;
  font-size: 0.85rem;
  margin: 0.25rem 0 0;
}

.actions {
  display: flex;
  gap: 0.4rem;
  flex-wrap: wrap;
}

.actions button,
.start-instance select,
.start-instance button {
  padding: 0.35rem 0.7rem;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  background: #fff;
  cursor: pointer;
}

.actions .delete,
.actions .unregister {
  color: #b91c1c;
  border-color: #fca5a5;
}

.actions .unregister {
  color: #334155;
  border-color: #cbd5e1;
}

.backend-config-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.start-instance {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.start-instance .hint,
.hint {
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

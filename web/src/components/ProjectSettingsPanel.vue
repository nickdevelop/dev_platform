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
import { useSessionsStore } from '../stores/sessions'
import BackendDiagnostics from './BackendDiagnostics.vue'
import ResourcesSection from './ResourcesSection.vue'

const props = defineProps<{ projectId: number }>()

const emit = defineEmits<{
  manage: [kind: 'move' | 'rename' | 'unregister' | 'delete', project: Project]
}>()

const projectsStore = useProjectsStore()
const agents = useAgentsStore()
const sessions = useSessionsStore()

const project = computed(() => projectsStore.byId(props.projectId) ?? null)
const selectedBackend = ref<BackendId | ''>('')
const scanning = ref(false)
const actionError = ref('')
const discoverNotice = ref('')

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

async function scan() {
  if (selectedBackend.value === '' || scanning.value) return
  scanning.value = true
  actionError.value = ''
  discoverNotice.value = ''
  // Discovery is project-level: the server starts or reuses the instance that
  // serves the backend, so the page never asks the user to pick one.
  const found = await sessions.discoverProjectSessions(props.projectId, selectedBackend.value)
  if (found === null) {
    actionError.value = sessions.error
  } else {
    discoverNotice.value = `已发现 ${found.length} 个原生会话。`
  }
  scanning.value = false
}

async function stopInstance(instanceId: string) {
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

// The scan control is disabled when the selected backend is disabled for the
// project, so the UI never offers an action the server will refuse.
const selectedBackendEnabled = computed(
  () => selectedBackend.value === '' || agents.backendEnabledFor(selectedBackend.value),
)

// sessionDiscovery is a backend capability; a backend without it has no scan.
const canScan = computed(
  () =>
    selectedBackend.value !== '' &&
    agents.capabilitiesFor(selectedBackend.value)?.sessionDiscovery === true,
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

    <BackendDiagnostics
      :diagnostics="agents.diagnostics"
      :instances="agents.instances"
      :capabilities-for="agents.capabilitiesFor"
      @stop="stopInstance"
    />

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

    <section class="scan-sessions" aria-label="扫描原生会话">
      <h2>扫描原生会话</h2>
      <div class="scan-controls">
        <label for="backend-select">Agent 后端</label>
        <select id="backend-select" v-model="selectedBackend">
          <option v-for="backend in agents.backends" :key="backend.backend" :value="backend.backend">
            {{ backend.backend }}
          </option>
        </select>
        <button
          v-if="canScan"
          type="button"
          class="scan"
          data-test="discover-sessions"
          :disabled="scanning || !selectedBackendEnabled"
          @click="scan"
        >
          扫描发现原生会话
        </button>
      </div>
      <p class="hint">扫描会把 Agent 原生保存的会话登记为任务；无需选择实例。</p>
    </section>

    <p v-if="scanning" class="loading">扫描中…</p>
    <p v-if="discoverNotice" class="discover-notice" role="status">{{ discoverNotice }}</p>

    <ResourcesSection :project-id="projectId" />
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
  color: var(--color-text-muted);
  font-size: 0.85rem;
  margin: 0.25rem 0 0;
}

.actions {
  display: flex;
  gap: 0.4rem;
  flex-wrap: wrap;
}

.actions button,
.scan-controls select,
.scan-controls button {
  padding: 0.35rem 0.7rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  cursor: pointer;
}

.actions .delete,
.actions .unregister {
  color: var(--color-danger);
  border-color: var(--color-danger-border);
}

.actions .unregister {
  color: var(--color-text);
  border-color: var(--color-border-strong);
}

.backend-config-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.scan-controls {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.scan-controls .hint,
.hint {
  flex-basis: 100%;
  color: var(--color-text-muted);
  font-size: 0.85rem;
  margin: 0;
}

.page-error {
  color: var(--color-danger);
}

.discover-notice {
  color: var(--color-success);
  font-size: 0.9rem;
}

.diagnostics-error {
  color: var(--color-warning);
}
</style>

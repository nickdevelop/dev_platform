<script setup lang="ts">
// Project settings, opened as a workbench tab: project facts, the project's
// Agent backends, session discovery and the danger-zone actions. The
// manage actions are handed up so the shell owns the single manage dialog.
//
// Instances are deliberately absent. They are how a backend serves the
// workbench, not something the user operates; what a user can act on is the
// backend's machine-level service, and that lives in platform settings — so this
// panel only points there when the service is down.
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import type { BackendId } from '../../api/agents'
import type { Project } from '../../api/projects'
import { useAgentsStore } from '../../stores/agents'
import { useProjectsStore } from '../../stores/projects'
import { useSessionsStore } from '../../stores/sessions'
import BackendDiagnostics from '../../shared/components/BackendDiagnostics.vue'

const props = defineProps<{ projectId: number }>()

const emit = defineEmits<{
  manage: [kind: 'move' | 'rename' | 'rename-name' | 'unregister', project: Project]
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
    agents.loadDiagnostics(),
    // One look per service-backed backend, on open, so the notice below tells
    // the truth about whether a session could start here at all. The platform
    // neither polls nor starts it.
    ...agents.serviceBackends.map((entry) => agents.detectService(entry.backend)),
  ])
}

// serviceDown is the one backend state a project cannot work around: when the
// selected backend's machine-level service is down, no session of that backend
// can start here. The panel says so and points at the only page that can
// change it.
const serviceDown = computed(() => {
  if (selectedBackend.value === '' || !agents.backendFor(selectedBackend.value)?.serviceSupported) {
    return false
  }
  const status = agents.services[selectedBackend.value]
  return status !== undefined && status !== null && status.state !== 'running'
})

// The service's name in the operator's words, e.g. "opencode service".
const serviceName = computed(() => `${agents.backendLabel(selectedBackend.value).toLowerCase()} service`)

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

// The scan control is disabled when the selected backend is disabled
// platform-wide, so the UI never offers an action the server will refuse.
const selectedBackendEnabled = computed(
  () => selectedBackend.value === '' || agents.backendEnabledFor(selectedBackend.value),
)

// sessionDiscovery is a backend capability; a backend without it has no scan.
const canScan = computed(
  () =>
    selectedBackend.value !== '' &&
    agents.capabilitiesFor(selectedBackend.value)?.sessionDiscovery === true,
)

function manage(kind: 'move' | 'rename' | 'rename-name' | 'unregister') {
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
        <button type="button" @click="manage('rename-name')">修改名称</button>
        <button type="button" @click="manage('rename')">重命名</button>
        <button type="button" @click="manage('move')">移动</button>
        <button type="button" class="unregister" @click="manage('unregister')">解除登记</button>
      </div>
    </header>

    <p v-if="agents.error || actionError" class="page-error" role="alert">
      {{ actionError || agents.error }}
    </p>
    <p v-else-if="agents.diagnosticsError" class="diagnostics-error" role="alert">
      {{ agents.diagnosticsError }}
    </p>

    <p v-if="serviceDown" class="service-down" role="alert" data-test="project-service-down">
      {{ serviceName }} 未启动，无法在此项目创建会话。请到
      <RouterLink :to="{ name: 'settings-agents' }">平台设置 → Agent</RouterLink>
      检测并启动。
    </p>

    <BackendDiagnostics :diagnostics="agents.diagnostics" />

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
      <p class="hint">扫描会把 Agent 原生保存的会话登记为会话。</p>
    </section>

    <p v-if="scanning" class="loading">扫描中…</p>
    <p v-if="discoverNotice" class="discover-notice" role="status">{{ discoverNotice }}</p>
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

.service-down {
  margin: 0;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--color-warning-border);
  border-radius: 8px;
  color: var(--color-warning);
  background: var(--color-surface);
}
</style>

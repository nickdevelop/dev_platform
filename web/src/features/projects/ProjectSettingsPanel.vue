<script setup lang="ts">
// Project settings, opened as a workbench tab: project facts, the project's
// Agent backends and the danger-zone actions. The manage actions are handed up
// so the shell owns the single manage dialog.
//
// Instances are deliberately absent. They are how a backend serves the
// workbench, not something the user operates; what a user can act on is the
// backend's machine-level service, and that lives in platform settings — so this
// panel only points there when the service is down.
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import type { Project } from '../../api/projects'
import { useAgentsStore } from '../../stores/agents'
import { useProjectsStore } from '../../stores/projects'
import BackendDiagnostics from '../../shared/components/BackendDiagnostics.vue'

const props = defineProps<{ projectId: number }>()

const emit = defineEmits<{
  manage: [kind: 'move' | 'rename' | 'rename-name' | 'unregister', project: Project]
}>()

const projectsStore = useProjectsStore()
const agents = useAgentsStore()

const project = computed(() => projectsStore.byId(props.projectId) ?? null)
const actionError = ref('')

onMounted(load)
watch(() => props.projectId, load)

async function load() {
  await agents.loadBackends()
  await Promise.all([
    agents.loadDiagnostics(),
    // One look per service-backed backend, on open, so the notice below tells
    // the truth about whether a session could start here at all. The platform
    // neither polls nor starts it.
    ...agents.serviceBackends.map((entry) => agents.detectService(entry.backend)),
  ])
}

// serviceDown reports if any service-backed backend needed by the platform is down.
const downService = computed(() => {
  for (const entry of agents.serviceBackends) {
    const status = agents.services[entry.backend]
    if (status !== undefined && status !== null && status.state !== 'running') {
      return entry
    }
  }
  return null
})

const serviceDown = computed(() => downService.value !== null)
const serviceName = computed(() =>
  downService.value ? `${agents.backendLabel(downService.value.backend).toLowerCase()} service` : '',
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

.actions button {
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

.page-error {
  color: var(--color-danger);
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

<script setup lang="ts">
// Left navigation tree: projects with their tasks (agent sessions). Only one
// project is expanded at a time; its tasks load on expansion. Tasks are shown
// only when the session store holds that project's list, so a stale list from a
// previously opened project is never rendered. Search matches project names and
// task titles; the active and expanded projects always stay visible, so the open
// selection is never hidden by a filter.
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useProjectsStore } from '../stores/projects'
import { useSessionsStore } from '../stores/sessions'
import BackendTag from './BackendTag.vue'

const props = defineProps<{
  activeProjectId: number | null
  activeSessionId: string | null
  search: string
}>()

const emit = defineEmits<{
  'select-project': [projectId: number]
  'select-session': [projectId: number, sessionId: string]
  'new-task': [projectId: number]
}>()

const projectsStore = useProjectsStore()
const sessionsStore = useSessionsStore()

const expandedId = ref<number | null>(props.activeProjectId)

const statusLabels: Record<string, string> = {
  running: '运行中',
}

// The task label is driven by the turn alone: a running turn shows 「运行中」
// and every other state is silent. A disconnected session carries no badge,
// because opening a task re-attaches it — a marker there would only ask the
// user to fix something the platform already repairs.
function statusText(entry: AgentSession): string {
  if (entry.turn === 'running') return statusLabels.running
  return ''
}

const expandedSessions = computed(() => {
  if (expandedId.value === null || sessionsStore.projectId !== expandedId.value) return []
  return sessionsStore.sessions
})

const query = computed(() => props.search.trim().toLowerCase())

const visibleSessions = computed(() => {
  if (query.value === '') return expandedSessions.value
  return expandedSessions.value.filter((entry) =>
    (entry.title || entry.id).toLowerCase().includes(query.value),
  )
})

const visibleProjects = computed(() => {
  if (query.value === '') return projectsStore.projects
  return projectsStore.projects.filter(
    (project) =>
      project.id === props.activeProjectId ||
      project.id === expandedId.value ||
      project.name.toLowerCase().includes(query.value),
  )
})

watch(
  () => props.activeProjectId,
  (projectId) => {
    expandedId.value = projectId
  },
)

watch(
  expandedId,
  (projectId) => {
    if (projectId !== null) void sessionsStore.loadSessions(projectId)
  },
  { immediate: true },
)

// A task whose turn is not the open one gets no SSE events, so the tree polls
// the session list while the expanded project has any running task. It stops as
// soon as the list reports none, and on unmount.
const hasRunningTasks = computed(() => expandedSessions.value.some((entry) => entry.turn === 'running'))

let pollTimer: ReturnType<typeof setInterval> | null = null

function stopPolling() {
  if (pollTimer !== null) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

watch(
  [expandedId, hasRunningTasks],
  ([projectId, running]) => {
    stopPolling()
    if (projectId !== null && running) {
      pollTimer = setInterval(() => {
        void sessionsStore.refreshSessions(projectId)
      }, 2000)
    }
  },
  { immediate: true },
)

onBeforeUnmount(stopPolling)

// Clicking the expanded project collapses it; clicking another project expands
// it and collapses the previous one. Expanding never navigates: the settings
// control is the only way into the project configuration.
function onProjectClick(projectId: number) {
  expandedId.value = projectId === expandedId.value ? null : projectId
}
</script>

<template>
  <ul class="project-tree" aria-label="项目与任务">
    <li v-for="project in visibleProjects" :key="project.id" class="project">
      <div class="project-row" :class="{ selected: project.id === props.activeProjectId }">
        <button
          type="button"
          class="project-toggle"
          :aria-expanded="expandedId === project.id"
          @click="onProjectClick(project.id)"
        >
          <span class="caret">{{ expandedId === project.id ? '▾' : '▸' }}</span>
          <span class="project-name">{{ project.name }}</span>
          <span v-if="expandedId === project.id" class="count">{{ expandedSessions.length }}</span>
        </button>
        <button
          type="button"
          class="project-settings"
          :aria-label="`项目设置：${project.name}`"
          title="项目设置"
          @click="emit('select-project', project.id)"
        >
          ⚙
        </button>
      </div>

      <ul v-if="expandedId === project.id" class="tasks">
        <li v-for="entry in visibleSessions" :key="entry.id" class="task" :class="{ selected: entry.id === props.activeSessionId }">
          <button
            type="button"
            class="task-open"
            @click="emit('select-session', project.id, entry.id)"
          >
            <span class="title">{{ entry.title || entry.id }}</span>
            <BackendTag :backend="entry.backend" />
            <span v-if="statusText(entry)" class="status">{{ statusText(entry) }}</span>
          </button>
        </li>
        <li v-if="expandedSessions.length === 0" class="empty">该项目还没有任务。</li>
        <li v-else-if="visibleSessions.length === 0" class="empty">没有匹配的任务</li>
        <li>
          <button type="button" class="new-task" @click="emit('new-task', project.id)">+ 新建任务</button>
        </li>
      </ul>
    </li>
    <li v-if="projectsStore.projects.length === 0" class="empty">还没有项目。</li>
    <li v-else-if="visibleProjects.length === 0" class="empty">没有匹配的项目。</li>
  </ul>
</template>

<style scoped>
.project-tree,
.tasks {
  list-style: none;
  margin: 0;
  padding: 0;
}

.project-row,
.project-toggle,
.task-open,
.new-task {
  display: flex;
  align-items: center;
  gap: calc(var(--space-unit) * 1.000);
  width: 100%;
  border: none;
  background: transparent;
  text-align: left;
  font: inherit;
  cursor: pointer;
  border-radius: 6px;
}

.project-row {
  display: flex;
  align-items: center;
  gap: calc(var(--space-unit) * 0.333);
  padding: 0 calc(var(--space-unit) * 0.667) 0 0;
  border-radius: 6px;
}

.project-toggle {
  flex: 1;
  min-width: 0;
  padding: calc(var(--space-unit) * 1.167) calc(var(--space-unit) * 1.333);
  font-weight: 600;
}

.project-settings {
  flex-shrink: 0;
  border: none;
  background: transparent;
  cursor: pointer;
  font: inherit;
  font-size: 0.929rem;
  line-height: 1;
  padding: calc(var(--space-unit) * 1.000);
  border-radius: 6px;
  color: var(--color-text-muted);
}

.project-row:hover,
.project-settings:hover,
.project-toggle:hover,
.task-open:hover {
  background: var(--color-surface-muted);
}

.project-row.selected {
  background: var(--color-info-soft);
  color: var(--color-primary-hover);
}

.project-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.count {
  color: var(--color-text-muted);
  font-size: 0.857rem;
  font-weight: 400;
}

.tasks {
  padding-left: calc(var(--space-unit) * 2.333);
}

.task {
  border-radius: 6px;
}

.task.selected .task-open {
  background: var(--color-surface-muted);
  font-weight: 600;
}

.task-open {
  padding: calc(var(--space-unit) * 1.000) calc(var(--space-unit) * 1.333);
  font-size: 0.929rem;
}

.title {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.status {
  color: var(--color-text-muted);
  font-size: 0.786rem;
  white-space: nowrap;
}

.new-task {
  padding: calc(var(--space-unit) * 0.667) calc(var(--space-unit) * 1.333);
  color: var(--color-primary-hover);
  font-size: 0.857rem;
}

.empty {
  padding: calc(var(--space-unit) * 0.667) calc(var(--space-unit) * 1.333);
  color: var(--color-text-muted);
  font-size: 0.857rem;
}
</style>

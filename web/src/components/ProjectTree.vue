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
// it and collapses the previous one.
function onProjectClick(projectId: number) {
  if (projectId === expandedId.value) {
    expandedId.value = null
    return
  }
  expandedId.value = projectId
  emit('select-project', projectId)
}
</script>

<template>
  <ul class="project-tree" aria-label="项目与任务">
    <li v-for="project in visibleProjects" :key="project.id" class="project">
      <button
        type="button"
        class="project-row"
        :class="{ selected: project.id === props.activeProjectId }"
        :aria-expanded="expandedId === project.id"
        @click="onProjectClick(project.id)"
      >
        <span class="caret">{{ expandedId === project.id ? '▾' : '▸' }}</span>
        <span class="project-name">{{ project.name }}</span>
        <span v-if="expandedId === project.id" class="count">{{ expandedSessions.length }}</span>
      </button>

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
.task-open,
.new-task {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  border: none;
  background: transparent;
  text-align: left;
  font: inherit;
  cursor: pointer;
  border-radius: 6px;
}

.project-row {
  padding: 7px 8px;
  font-weight: 600;
}

.project-row:hover,
.task-open:hover {
  background: #f1f3f6;
}

.project-row.selected {
  background: #e8f0ff;
  color: #1d4ed8;
}

.project-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.count {
  color: #8a93a3;
  font-size: 12px;
  font-weight: 400;
}

.tasks {
  padding-left: 14px;
}

.task {
  border-radius: 6px;
}

.task.selected .task-open {
  background: #eef2f7;
  font-weight: 600;
}

.task-open {
  padding: 6px 8px;
  font-size: 13px;
}

.title {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.status {
  color: #64748b;
  font-size: 11px;
  white-space: nowrap;
}

.new-task {
  padding: 4px 8px;
  color: #1d4ed8;
  font-size: 12px;
}

.empty {
  padding: 4px 8px;
  color: #8a93a3;
  font-size: 12px;
}
</style>

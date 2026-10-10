<script setup lang="ts">
// Left navigation tree: projects with their tasks (agent sessions). Only one
// project is expanded at a time; its tasks load on expansion. Tasks are shown
// only when the session store holds that project's list, so a stale list from a
// previously opened project is never rendered. Search filters tasks by title
// and never changes the selection.
import { computed, ref, watch } from 'vue'
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
  active: '运行中',
  stopped: '空闲',
  disconnected: '断开',
}

const expandedSessions = computed(() => {
  if (expandedId.value === null || sessionsStore.projectId !== expandedId.value) return []
  return sessionsStore.sessions
})

const visibleSessions = computed(() => {
  const query = props.search.trim().toLowerCase()
  if (query === '') return expandedSessions.value
  return expandedSessions.value.filter((entry) =>
    (entry.title || entry.id).toLowerCase().includes(query),
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
    <li v-for="project in projectsStore.projects" :key="project.id" class="project">
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
            <span class="status">{{ statusLabels[entry.status] ?? entry.status }}</span>
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

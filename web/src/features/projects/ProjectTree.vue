<script setup lang="ts">
// Left navigation tree: projects with their sessions (agent sessions). Multiple
// projects can stay expanded at once; each expanded project's sessions load
// independently into a per-project cache, so opening one project never clears
// another's list. Sessions are ordered by last interaction (newest first) and
// paginated per project: the list shows the newest 15 and a 「更多」 button
// appends the next 15. Search matches project names and session titles; the
// active and expanded projects always stay visible, so the open selection is
// never hidden by a filter.
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { listProjectSessions, type AgentSession } from '../../api/sessions'
import { useProjectsStore } from '../../stores/projects'
import { useSessionsStore } from '../../stores/sessions'
import BackendTag from '../../shared/components/BackendTag.vue'
import RunningIndicator from '../../shared/components/RunningIndicator.vue'

const props = defineProps<{
  activeProjectId: number | null
  activeSessionId: string | null
  search: string
}>()

const emit = defineEmits<{
  // A project's settings are a workbench tab, not a page the tree navigates to.
  'open-settings': [projectId: number]
  'select-session': [projectId: number, sessionId: string]
  'new-session': [projectId: number]
}>()

const projectsStore = useProjectsStore()
const sessionsStore = useSessionsStore()

// Multiple projects stay expanded at once: an array (reactive) instead of a
// single id. The active project is added, never replaces the set.
const expandedIds = ref<number[]>(
  props.activeProjectId === null ? [] : [props.activeProjectId],
)

function isExpanded(projectId: number): boolean {
  return expandedIds.value.includes(projectId)
}

// Per-project session cache: each expanded project loads its own list, so lists
// never clobber each other like a single shared store list would.
const sessionsByProject = ref<Record<number, AgentSession[]>>({})
const loadFailed = ref<Record<number, boolean>>({})

async function loadProject(projectId: number) {
  try {
    const list = await listProjectSessions(projectId)
    sessionsByProject.value = { ...sessionsByProject.value, [projectId]: list.sessions }
    loadFailed.value = { ...loadFailed.value, [projectId]: false }
  } catch {
    loadFailed.value = { ...loadFailed.value, [projectId]: true }
  }
}

function sessionsFor(projectId: number): AgentSession[] {
  return sessionsByProject.value[projectId] ?? []
}

// The session label is driven by the turn alone: a running turn shows an
// animated marker before the name and every other state is silent. A
// disconnected session carries no badge, because opening a session re-attaches
// it — a marker there would only ask the user to fix something the platform
// already repairs.
function isRunning(entry: AgentSession): boolean {
  return entry.turn === 'running'
}

// Sessions are ordered by last interaction (the session's updated-at, which the
// backend bumps on send/continue/status change), newest first. Ties keep the
// server's creation order, because the sort below is stable.
function sortedFor(projectId: number): AgentSession[] {
  return [...sessionsFor(projectId)].sort(
    (a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt),
  )
}

const query = computed(() => props.search.trim().toLowerCase())

// Display fallback shared by the row and the search filter: the platform id
// must never surface as a name.
const untitledLabel = '未命名会话'

// Pagination per project: only the newest PAGE_SIZE sessions render at first;
// 「更多」 appends the next page. Counters reset when the search changes.
const PAGE_SIZE = 15
const visibleCountByProject = ref<Record<number, number>>({})

function visibleCountFor(projectId: number): number {
  return visibleCountByProject.value[projectId] ?? PAGE_SIZE
}

watch(query, () => {
  visibleCountByProject.value = {}
})

function filteredFor(projectId: number): AgentSession[] {
  const sorted = sortedFor(projectId)
  if (query.value === '') return sorted
  return sorted.filter((entry) =>
    (entry.title || untitledLabel).toLowerCase().includes(query.value),
  )
}

function visibleSessionsFor(projectId: number): AgentSession[] {
  return filteredFor(projectId).slice(0, visibleCountFor(projectId))
}

function hasMoreSessions(projectId: number): boolean {
  return filteredFor(projectId).length > visibleSessionsFor(projectId).length
}

function showMoreSessions(projectId: number) {
  visibleCountByProject.value = {
    ...visibleCountByProject.value,
    [projectId]: visibleCountFor(projectId) + PAGE_SIZE,
  }
}

const visibleProjects = computed(() => {
  if (query.value === '') return projectsStore.projects
  return projectsStore.projects.filter(
    (project) =>
      project.id === props.activeProjectId ||
      isExpanded(project.id) ||
      project.name.toLowerCase().includes(query.value),
  )
})

watch(
  () => props.activeProjectId,
  (projectId) => {
    if (projectId !== null && !isExpanded(projectId)) {
      expandedIds.value = [...expandedIds.value, projectId]
    }
  },
)

watch(
  expandedIds,
  (ids) => {
    for (const projectId of ids) {
      if (!(projectId in sessionsByProject.value)) void loadProject(projectId)
    }
  },
  { immediate: true },
)

// A session created in the main pane — or deleted from it — is not in any list the
// tree holds, and the tree polls only while a turn runs. The store records every
// change it makes to the session index; each expanded project is refetched then.
watch(
  () => sessionsStore.revision,
  () => {
    for (const projectId of expandedIds.value) void loadProject(projectId)
  },
)

// The 「运行中」 label is read from the cached list, so a turn started in the
// conversation pane would never show: the tree polls a project only once its own
// list already says a session is running. When the store reports a turn starting or
// ending in the project it is showing, that project is refetched — once per
// change of turns, not once per event — and the tree's own polling carries on.
const storeTurns = computed(() => sessionsStore.sessions.map((entry) => entry.id + ':' + entry.turn).join(','))

watch(storeTurns, () => {
  const projectId = sessionsStore.projectId
  if (projectId !== null && isExpanded(projectId)) void loadProject(projectId)
})

// Sessions whose turn is running get no SSE events in the tree, so the tree polls
// every expanded project that has any running session. It stops for a project as
// soon as its list reports none, and entirely on unmount.
function projectHasRunning(projectId: number): boolean {
  return sessionsFor(projectId).some((entry) => entry.turn === 'running')
}

const anyRunning = computed(() => expandedIds.value.some((id) => projectHasRunning(id)))

let pollTimer: ReturnType<typeof setInterval> | null = null

function stopPolling() {
  if (pollTimer !== null) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

watch(
  [expandedIds, anyRunning],
  ([ids, running]) => {
    stopPolling()
    if ((ids as number[]).length > 0 && running) {
      pollTimer = setInterval(() => {
        for (const projectId of expandedIds.value) {
          if (projectHasRunning(projectId)) void loadProject(projectId)
        }
      }, 2000)
    }
  },
  { immediate: true },
)

onBeforeUnmount(stopPolling)

// Clicking a project toggles only that project; other expanded projects stay
// open. Toggling never navigates: the settings control is the only way into
// the project configuration.
function onProjectClick(projectId: number) {
  expandedIds.value = isExpanded(projectId)
    ? expandedIds.value.filter((id) => id !== projectId)
    : [...expandedIds.value, projectId]
}

function retryProject(projectId: number) {
  void loadProject(projectId)
}
</script>

<template>
  <ul class="project-tree" aria-label="项目与会话">
    <li v-for="project in visibleProjects" :key="project.id" class="project">
      <div class="project-row" :class="{ selected: project.id === props.activeProjectId }">
        <button
          type="button"
          class="project-toggle"
          :aria-expanded="isExpanded(project.id)"
          @click="onProjectClick(project.id)"
        >
          <span class="caret">{{ isExpanded(project.id) ? '▾' : '▸' }}</span>
          <span class="project-name">{{ project.name }}</span>
          <span v-if="isExpanded(project.id)" class="count">{{ sessionsFor(project.id).length }}</span>
        </button>
        <button
          type="button"
          class="project-new-session"
          :aria-label="`新建会话：${project.name}`"
          title="新建会话"
          @click="emit('new-session', project.id)"
        >
          ＋
        </button>
        <button
          type="button"
          class="project-settings"
          :aria-label="`项目设置：${project.name}`"
          title="项目设置"
          @click="emit('open-settings', project.id)"
        >
          ⚙
        </button>
      </div>

      <ul v-if="isExpanded(project.id)" class="sessions">
        <li v-for="entry in visibleSessionsFor(project.id)" :key="entry.id" class="session" :class="{ selected: entry.id === props.activeSessionId }">
          <button
            type="button"
            class="session-open"
            @click="emit('select-session', project.id, entry.id)"
          >
            <RunningIndicator v-if="isRunning(entry)" />
            <span class="title">{{ entry.title || untitledLabel }}</span>
            <BackendTag :backend="entry.backend" />
          </button>
        </li>
        <li v-if="loadFailed[project.id]" class="empty">会话加载失败。<button type="button" class="load-more" @click="retryProject(project.id)">重试</button></li>
        <li v-else-if="sessionsFor(project.id).length === 0" class="empty">该项目还没有会话。</li>
        <li v-else-if="visibleSessionsFor(project.id).length === 0" class="empty">没有匹配的会话</li>
        <li v-if="hasMoreSessions(project.id)">
          <button type="button" class="load-more" @click="showMoreSessions(project.id)">更多</button>
        </li>
      </ul>
    </li>
    <li v-if="projectsStore.projects.length === 0" class="empty">还没有项目。</li>
    <li v-else-if="visibleProjects.length === 0" class="empty">没有匹配的项目。</li>
  </ul>
</template>

<style scoped>
.project-tree,
.sessions {
  list-style: none;
  margin: 0;
  padding: 0;
}

.project-row,
.project-toggle,
.session-open {
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

.project-new-session,
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
.project-new-session:hover,
.project-settings:hover,
.project-toggle:hover,
.session-open:hover {
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

.sessions {
  padding-left: calc(var(--space-unit) * 2.333);
}

.session {
  border-radius: 6px;
}

.session.selected .session-open {
  background: var(--color-surface-muted);
  font-weight: 600;
}

.session-open {
  padding: calc(var(--space-unit) * 1.000) calc(var(--space-unit) * 1.333);
  font-size: 0.929rem;
}

.title {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.load-more {
  width: 100%;
  padding: calc(var(--space-unit) * 0.667) calc(var(--space-unit) * 1.333);
  border: none;
  background: transparent;
  text-align: left;
  font: inherit;
  font-size: 0.857rem;
  cursor: pointer;
  border-radius: 6px;
  color: var(--color-primary-hover);
}

.load-more:hover {
  background: var(--color-surface-muted);
}

.empty {
  padding: calc(var(--space-unit) * 0.667) calc(var(--space-unit) * 1.333);
  color: var(--color-text-muted);
  font-size: 0.857rem;
}
</style>

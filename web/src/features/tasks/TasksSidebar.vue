<script setup lang="ts">
// The task center's middle column: the task list and its filters.
// Selecting a task navigates to the task-detail route; clicking "新建" navigates to task-new.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { Task, TaskType } from '../../api/tasks'
import { useOpenTabsStore } from '../../stores/openTabs'
import { useProjectsStore } from '../../stores/projects'
import { useTasksStore } from '../../stores/tasks'
import {
  getRunStatusBadgeClass,
  getRunStatusLabel,
  getScheduleTypeLabel,
  getTaskTypeLabel,
} from './status'
import { UiSelectMenu, type SelectMenuItem } from '../../shared/ui'

const route = useRoute()
const router = useRouter()
const tasksStore = useTasksStore()
const projectsStore = useProjectsStore()
const openTabs = useOpenTabsStore()

const searchKeyword = ref('')
const selectedProjectFilter = ref<number | undefined>(undefined)
const selectedTypeFilter = ref<TaskType | undefined>(undefined)

const projectFilterOptions = computed<SelectMenuItem<number | undefined>[]>(() => {
  const items: SelectMenuItem<number | undefined>[] = [
    { value: undefined, label: '全部项目' },
  ]
  const list = Array.isArray(projectsStore.projects) ? projectsStore.projects : []
  for (const p of list) {
    items.push({ value: p.id, label: p.name })
  }
  return items
})

const typeFilterOptions = computed<SelectMenuItem<TaskType | undefined>[]>(() => [
  { value: undefined, label: '全部类型' },
  { value: 'standard', label: '普通任务' },
  { value: 'flow', label: '流程任务' },
])

onMounted(async () => {
  await Promise.all([tasksStore.loadTasks(), projectsStore.load()])
})

const isNewRoute = computed(() => route.name === 'task-new')

const activeTaskId = computed<number | null>(() => {
  if (isNewRoute.value) return null
  return route.params.taskId === undefined ? null : Number(route.params.taskId)
})

const filteredTasks = computed(() => {
  return tasksStore.tasks.filter((t) => {
    if (searchKeyword.value.trim()) {
      const q = searchKeyword.value.trim().toLowerCase()
      const matchTitle = t.title.toLowerCase().includes(q)
      const matchDesc = t.description?.toLowerCase().includes(q)
      if (!matchTitle && !matchDesc) return false
    }
    if (selectedProjectFilter.value && t.projectId !== selectedProjectFilter.value) {
      return false
    }
    if (selectedTypeFilter.value && t.taskType !== selectedTypeFilter.value) {
      return false
    }
    return true
  })
})

// Update tab strip title with readable task name
watch(
  () => tasksStore.tasks.map((entry) => `${entry.id}:${entry.title}`).join(','),
  () => {
    for (const task of tasksStore.tasks) {
      openTabs.updateTaskTitle(task.id, task.title)
    }
  },
)

function selectTask(id: number) {
  void router.push({ name: 'task-detail', params: { taskId: id } })
}

function openCreateView() {
  void router.push({ name: 'task-new' })
}

function getProjectName(pid: number) {
  return projectsStore.byId(pid)?.name ?? `项目 #${pid}`
}
</script>

<template>
  <div class="tasks-sidebar" aria-label="任务列表">
    <div class="sidebar-actions">
      <button
        type="button"
        class="action-btn new-task-btn"
        data-test="btn-new-task"
        @click="openCreateView"
      >
        + 新建
      </button>
    </div>

    <input
      v-model="searchKeyword"
      type="search"
      class="search search-input"
      placeholder="搜索任务名称或描述"
      aria-label="搜索任务名称或描述"
    />

    <div class="sidebar-filters">
      <UiSelectMenu
        v-model="selectedProjectFilter"
        class="filter-select project-filter flex-1 min-w-0"
        aria-label="按项目筛选"
        :options="projectFilterOptions"
        size="sm"
        placeholder="全部项目"
      />

      <UiSelectMenu
        v-model="selectedTypeFilter"
        class="filter-select type-filter flex-1 min-w-0"
        aria-label="按类型筛选"
        :options="typeFilterOptions"
        size="sm"
        placeholder="全部类型"
      />
    </div>

    <!-- Task items list: styled consistently with ProjectSessionItem -->
    <div class="task-list">
      <p v-if="filteredTasks.length === 0" class="empty-list">暂无匹配任务</p>
      <div
        v-for="t in filteredTasks"
        :key="t.id"
        class="task-card"
        :class="{ active: activeTaskId === t.id }"
        data-test="task-card"
        tabindex="0"
        role="button"
        :aria-label="t.title"
        @click="selectTask(t.id)"
        @keydown.enter.prevent="selectTask(t.id)"
        @keydown.space.prevent="selectTask(t.id)"
      >
        <!-- Row 1: Badges + Title -->
        <div class="task-row-top">
          <span class="type-pill" :class="t.taskType">
            {{ getTaskTypeLabel(t.taskType) }}
          </span>
          <span class="status-badge" :class="getRunStatusBadgeClass(t.lastRunStatus)">
            {{ getRunStatusLabel(t.lastRunStatus) }}
          </span>
          <span class="task-title" :title="t.title">{{ t.title }}</span>
        </div>

        <!-- Row 2: Meta (Project name + Schedule tag) -->
        <div class="task-row-bottom">
          <span class="meta-project" :title="getProjectName(t.projectId)">
            {{ getProjectName(t.projectId) }}
          </span>
          <span class="meta-schedule">
            {{ getScheduleTypeLabel(t.scheduleType) }}
          </span>
          <span v-if="t.scheduleEnabled && t.scheduleType === 'cron'" class="schedule-tag" :title="`周期: ${t.cronExpr}`">
            <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="1.8">
              <circle cx="8" cy="8" r="6.5" />
              <polyline points="8 4 8 8 11 10" />
            </svg>
            <span>{{ t.cronExpr }}</span>
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tasks-sidebar {
  display: flex;
  flex-direction: column;
  min-height: 0;
  flex: 1;
  overflow: hidden;
}

.sidebar-actions {
  display: flex;
  gap: 6px;
  padding: 0 12px 8px 12px;
}

.action-btn,
.new-task-btn {
  flex: 1;
  width: 100%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 26px;
  border: 1px solid var(--color-border);
  background: var(--color-surface);
  color: var(--color-text);
  border-radius: var(--radius, 4px);
  padding: 0 8px;
  font: inherit;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  transition: color 0.12s ease-out, border-color 0.12s ease-out, background-color 0.12s ease-out;
  white-space: nowrap;
}

.action-btn:hover,
.new-task-btn:hover {
  background: var(--color-surface-muted);
  border-color: var(--color-border-strong);
  color: var(--color-text);
}

.search,
.search-input {
  margin: 0 12px 6px 12px;
  padding: 4px 10px;
  height: 28px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius, 4px);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
  font-size: 12px;
  box-sizing: border-box;
  outline: none;
  transition: border-color 0.12s ease-out;
}

.search:focus,
.search-input:focus {
  border-color: var(--color-accent, #4a8ad6);
}

.search::placeholder,
.search-input::placeholder {
  color: var(--color-text-muted);
}

.sidebar-filters {
  display: flex;
  gap: 6px;
  padding: 0 12px 8px 12px;
}

.filter-select {
  flex: 1;
  min-width: 0;
}

.task-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 0 8px 16px 8px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.empty-list {
  font-size: 12px;
  color: var(--color-text-muted);
  text-align: center;
  margin: 24px 0;
}

.task-card {
  padding: 6px 8px;
  border-radius: var(--radius, 4px);
  border-left: 2px solid transparent;
  background: transparent;
  color: var(--color-text);
  cursor: pointer;
  outline: none;
  transition: background-color 0.12s ease-out, border-color 0.12s ease-out;
  user-select: none;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.task-card:hover {
  background: var(--color-surface-muted);
}

.task-card.active {
  border-left-color: var(--color-accent, #4a8ad6);
  background: var(--color-surface-muted);
  font-weight: 500;
}

.task-row-top {
  display: flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
}

.type-pill {
  font-size: 10px;
  font-weight: 500;
  padding: 0 4px;
  height: 16px;
  line-height: 16px;
  border-radius: 3px;
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border);
  color: var(--color-text-muted);
  flex-shrink: 0;
}

.type-pill.flow {
  border-color: color-mix(in srgb, var(--color-accent, #4a8ad6) 40%, transparent);
  color: var(--color-accent, #4a8ad6);
}

.status-badge {
  font-size: 10px;
  font-weight: 500;
  padding: 0 4px;
  height: 16px;
  line-height: 16px;
  border-radius: 3px;
  border: 1px solid var(--color-border);
  background: var(--color-surface-muted);
  color: var(--color-text-muted);
  flex-shrink: 0;
}

.status-badge.badge-running {
  background: color-mix(in srgb, var(--color-accent, #4a8ad6) 15%, transparent);
  border-color: color-mix(in srgb, var(--color-accent, #4a8ad6) 35%, transparent);
  color: var(--color-accent, #4a8ad6);
}

.status-badge.badge-success {
  background: color-mix(in srgb, var(--color-success, #44a282) 15%, transparent);
  border-color: color-mix(in srgb, var(--color-success, #44a282) 35%, transparent);
  color: var(--color-success, #44a282);
}

.status-badge.badge-failed {
  background: color-mix(in srgb, var(--color-danger, #e5566f) 15%, transparent);
  border-color: color-mix(in srgb, var(--color-danger, #e5566f) 35%, transparent);
  color: var(--color-danger, #e5566f);
}

.task-title {
  font-size: 12px;
  font-weight: 500;
  color: var(--color-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
}

.task-row-bottom {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  font-size: 11px;
  color: var(--color-text-muted);
}

.meta-project {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 120px;
}

.meta-schedule {
  flex-shrink: 0;
}

.schedule-tag {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 10px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-variant-numeric: tabular-nums;
  color: var(--color-text-muted);
  flex-shrink: 0;
}
</style>

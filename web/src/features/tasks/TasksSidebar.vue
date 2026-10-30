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
  gap: calc(var(--space-unit) * 1.0);
  padding: 0 calc(var(--space-unit) * 2.0) calc(var(--space-unit) * 1.333);
}

.action-btn,
.new-task-btn {
  flex: 1;
  width: 100%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--color-border-strong);
  background: var(--color-surface);
  color: var(--color-text);
  border-radius: var(--radius);
  padding: calc(var(--space-unit) * 0.667) calc(var(--space-unit) * 1.333);
  font: inherit;
  font-size: 0.857rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
}

.action-btn:hover,
.new-task-btn:hover {
  background: var(--color-surface-muted);
  border-color: var(--color-primary);
  color: var(--color-primary);
  opacity: 1;
}

.search,
.search-input {
  margin: 0 calc(var(--space-unit) * 2.0) calc(var(--space-unit) * 1.0);
  padding: calc(var(--space-unit) * 1.0) calc(var(--space-unit) * 1.667);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
  font-size: 0.929rem;
  box-sizing: border-box;
  outline: none;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}

.search:focus,
.search-input:focus {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px var(--color-info-soft);
}

.search::placeholder,
.search-input::placeholder {
  color: var(--color-text-muted);
}

.sidebar-filters {
  display: flex;
  gap: calc(var(--space-unit) * 1.0);
  padding: 0 calc(var(--space-unit) * 2.0);
  margin-bottom: calc(var(--space-unit) * 1.333);
}

.filter-select {
  flex: 1;
  min-width: 0;
  padding: calc(var(--space-unit) * 0.667) calc(var(--space-unit) * 1.0);
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
  font-size: 0.8rem;
  outline: none;
  transition: border-color 0.15s ease;
}

.filter-select:focus {
  border-color: var(--color-primary);
}

.task-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 0 calc(var(--space-unit) * 1.5) calc(var(--space-unit) * 2.0);
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 0.5);
}

.empty-list {
  font-size: 0.857rem;
  color: var(--color-text-muted);
  text-align: center;
  margin: calc(var(--space-unit) * 2.0) 0;
}

.task-card {
  padding: calc(var(--space-unit) * 0.75) calc(var(--space-unit) * 1.0);
  border: 1px solid transparent;
  border-radius: var(--radius);
  background: transparent;
  color: var(--color-text);
  cursor: pointer;
  outline: none;
  transition: all 0.15s ease;
  user-select: none;
}

.task-card:hover {
  border-color: var(--color-border);
  background: var(--color-surface-muted);
}

.task-card.active {
  border-color: var(--color-primary);
  background: var(--color-surface-muted);
  box-shadow: 0 0 6px var(--color-primary-soft, rgba(59, 130, 246, 0.2));
  font-weight: 500;
}

.card-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: calc(var(--space-unit) * 0.4);
}

.type-pill {
  font-size: 0.72rem;
  font-weight: 600;
  padding: 0.12rem 0.45rem;
  border-radius: 4px;
  background: rgba(59, 130, 246, 0.12);
  color: #3b82f6;
}

.type-pill.flow {
  background: var(--color-primary-soft);
  color: var(--color-accent);
}

.status-badge {
  font-size: 0.72rem;
  font-weight: 500;
  padding: 0.12rem 0.45rem;
  border-radius: 4px;
}

.badge-running {
  background: rgba(59, 130, 246, 0.15);
  color: #2563eb;
}

.badge-success {
  background: rgba(16, 185, 129, 0.15);
  color: #10b981;
}

.badge-failed {
  background: rgba(239, 68, 68, 0.15);
  color: #ef4444;
}

.badge-idle {
  background: rgba(156, 163, 175, 0.15);
  color: var(--color-text-muted);
}

.card-title {
  font-size: 0.929rem;
  font-weight: 500;
  color: var(--color-text);
  margin: 0 0 calc(var(--space-unit) * 0.3) 0;
  line-height: 1.35;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 0.75rem;
  color: var(--color-text-muted);
}

.schedule-tag {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  margin-top: calc(var(--space-unit) * 0.3);
  font-size: 0.75rem;
  color: var(--color-accent);
}
</style>

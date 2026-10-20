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

const route = useRoute()
const router = useRouter()
const tasksStore = useTasksStore()
const projectsStore = useProjectsStore()
const openTabs = useOpenTabsStore()

const searchKeyword = ref('')
const selectedProjectFilter = ref<number | undefined>(undefined)
const selectedTypeFilter = ref<TaskType | undefined>(undefined)

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
        class="new-task-btn"
        data-test="btn-new-task"
        @click="openCreateView"
      >
        + 新建
      </button>
    </div>

    <input
      v-model="searchKeyword"
      type="search"
      class="search-input"
      placeholder="搜索任务名称或描述"
      aria-label="搜索任务名称或描述"
    />

    <div class="sidebar-filters">
      <select
        v-model="selectedProjectFilter"
        class="filter-select project-filter"
        aria-label="按项目筛选"
      >
        <option :value="undefined">全部项目</option>
        <option v-for="p in projectsStore.projects" :key="p.id" :value="p.id">
          {{ p.name }}
        </option>
      </select>

      <select
        v-model="selectedTypeFilter"
        class="filter-select type-filter"
        aria-label="按类型筛选"
      >
        <option :value="undefined">全部类型</option>
        <option value="standard">普通任务</option>
        <option value="flow">流程任务</option>
      </select>
    </div>

    <!-- Task items list -->
    <div class="task-list">
      <p v-if="filteredTasks.length === 0" class="empty-list">暂无匹配任务</p>
      <div
        v-for="t in filteredTasks"
        :key="t.id"
        class="task-card"
        :class="{ active: activeTaskId === t.id }"
        data-test="task-card"
        @click="selectTask(t.id)"
      >
        <div class="card-top">
          <span class="type-pill" :class="t.taskType">
            {{ getTaskTypeLabel(t.taskType) }}
          </span>
          <span class="status-badge" :class="getRunStatusBadgeClass(t.lastRunStatus)">
            {{ getRunStatusLabel(t.lastRunStatus) }}
          </span>
        </div>

        <h4 class="card-title">{{ t.title }}</h4>

        <div class="card-meta">
          <span class="meta-project">{{ getProjectName(t.projectId) }}</span>
          <span class="meta-schedule">
            {{ getScheduleTypeLabel(t.scheduleType) }}
          </span>
        </div>

        <div v-if="t.scheduleEnabled && t.scheduleType === 'cron'" class="schedule-tag">
          <span class="tag-icon">⏰</span>
          <span>周期: {{ t.cronExpr }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tasks-sidebar {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 0 1rem 1rem;
  background: var(--color-surface);
  border-right: 1px solid var(--color-border);
  overflow: hidden;
}

.sidebar-actions {
  display: flex;
  padding-top: calc(var(--space-unit, 6px) * 1.8);
  margin-bottom: 0.6rem;
}

.new-task-btn {
  width: 100%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0.45rem 0.75rem;
  font-size: 0.85rem;
  font-weight: 600;
  background: var(--color-primary, #3b82f6);
  color: #fff;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  transition: opacity 0.15s;
}

.new-task-btn:hover {
  opacity: 0.9;
}

.search-input {
  width: 100%;
  padding: 0.45rem 0.65rem;
  margin-bottom: 0.6rem;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  background: var(--color-surface-muted);
  color: var(--color-text);
  font-size: 0.85rem;
}

.search-input:focus {
  outline: none;
  border-color: var(--color-primary, #3b82f6);
}

.sidebar-filters {
  display: flex;
  gap: 0.5rem;
  margin-bottom: 0.75rem;
}

.filter-select {
  flex: 1;
  padding: 0.35rem 0.5rem;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  background: var(--color-surface);
  color: var(--color-text);
  font-size: 0.8rem;
}

.task-list {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}

.empty-list {
  font-size: 0.85rem;
  color: var(--color-text-muted);
  text-align: center;
  margin-top: 2rem;
}

.task-card {
  padding: 0.75rem;
  border: 1px solid var(--color-border);
  border-radius: 8px;
  background: var(--color-surface-muted);
  cursor: pointer;
  transition: all 0.15s ease;
}

.task-card:hover {
  border-color: var(--color-primary, #3b82f6);
  background: var(--color-surface);
}

.task-card.active {
  border-color: var(--color-primary, #3b82f6);
  background: var(--color-surface);
  box-shadow: 0 0 0 1px var(--color-primary, #3b82f6);
}

.card-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.4rem;
}

.type-pill {
  font-size: 0.72rem;
  font-weight: 600;
  padding: 0.15rem 0.45rem;
  border-radius: 4px;
  background: rgba(59, 130, 246, 0.12);
  color: #3b82f6;
}

.type-pill.flow {
  background: rgba(168, 85, 247, 0.12);
  color: #a855f7;
}

.status-badge {
  font-size: 0.72rem;
  font-weight: 500;
  padding: 0.15rem 0.45rem;
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
  font-size: 0.92rem;
  font-weight: 600;
  color: var(--color-text);
  margin: 0 0 0.4rem 0;
  line-height: 1.35;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 0.78rem;
  color: var(--color-text-muted);
}

.schedule-tag {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  margin-top: 0.4rem;
  font-size: 0.75rem;
  color: #8b5cf6;
}
</style>

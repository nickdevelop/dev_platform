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
    <!-- Top dedicated action button -->
    <div class="sidebar-actions">
      <button
        type="button"
        class="action-btn new-task-btn"
        data-test="btn-new-task"
        @click="openCreateView"
      >
        <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
          <line x1="8" y1="3" x2="8" y2="13" />
          <line x1="3" y1="8" x2="13" y2="8" />
        </svg>
        <span>新建任务</span>
      </button>
    </div>

    <!-- Search input -->
    <input
      v-model="searchKeyword"
      type="search"
      class="search search-input"
      placeholder="搜索任务名称或描述"
      aria-label="搜索任务名称或描述"
    />

    <!-- Filter dropdowns -->
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

    <!-- Task list items -->
    <div class="task-list" role="list">
      <p v-if="filteredTasks.length === 0" class="empty-list">
        {{ searchKeyword.trim() ? '暂无匹配任务' : '暂无任务，点击上方新建' }}
      </p>

      <div
        v-for="t in filteredTasks"
        :key="t.id"
        class="task-card"
        :class="[
          { active: activeTaskId === t.id },
          `status-${t.lastRunStatus || 'idle'}`
        ]"
        data-test="task-card"
        tabindex="0"
        role="button"
        :aria-label="t.title"
        @click="selectTask(t.id)"
        @keydown.enter.prevent="selectTask(t.id)"
        @keydown.space.prevent="selectTask(t.id)"
      >
        <!-- Row 1: Title + Status Pill -->
        <div class="task-row-top">
          <span class="task-title" :title="t.title">{{ t.title }}</span>
          <span
            class="status-badge"
            :class="getRunStatusBadgeClass(t.lastRunStatus)"
          >
            {{ getRunStatusLabel(t.lastRunStatus) }}
          </span>
        </div>

        <!-- Row 2: Type pill, Project name & Schedule tag -->
        <div class="task-row-bottom">
          <div class="flex items-center gap-1.5 min-w-0 overflow-hidden">
            <span class="type-pill" :class="t.taskType">
              {{ getTaskTypeLabel(t.taskType) }}
            </span>
            <span class="meta-project" :title="getProjectName(t.projectId)">
              {{ getProjectName(t.projectId) }}
            </span>
          </div>

          <div class="flex items-center gap-1 shrink-0">
            <span
              v-if="t.scheduleEnabled && t.scheduleType === 'cron'"
              class="schedule-tag"
              :title="`Cron: ${t.cronExpr}`"
            >
              <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
                <circle cx="8" cy="8" r="6" />
                <polyline points="8 4.5 8 8 10.5 9.5" />
              </svg>
              <span>{{ t.cronExpr }}</span>
            </span>
            <span v-else class="meta-schedule text-slate-500">
              {{ getScheduleTypeLabel(t.scheduleType) }}
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
@import './tasks-view.css';
</style>

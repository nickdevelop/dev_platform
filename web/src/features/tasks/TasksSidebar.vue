<script setup lang="ts">
// The task center's middle column: the task list and its filters, rendered by
// the shell in place of the project tree when the tasks area is open. Selecting
// a task navigates to the task-detail route, so the detail pane opens — and is
// labelled by — the third column's tab strip; the tab's title follows the task
// list once it loads. Task creation is managed from this column's dialog, and
// saving navigates to the new task.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { FlowType, Task, TaskStatus } from '../../api/tasks'
import { useOpenTabsStore } from '../../stores/openTabs'
import { useProjectsStore } from '../../stores/projects'
import { useTasksStore } from '../../stores/tasks'
import { statusOptions, getStatusBadgeClass, getStatusLabel } from './status'
import TaskManageDialog from './TaskManageDialog.vue'

const route = useRoute()
const router = useRouter()
const tasksStore = useTasksStore()
const projectsStore = useProjectsStore()
const openTabs = useOpenTabsStore()

const searchKeyword = ref('')
const selectedProjectFilter = ref<number | undefined>(undefined)
const selectedFlowFilter = ref<FlowType | undefined>(undefined)
const selectedStatusFilter = ref<TaskStatus | undefined>(undefined)

const showManageDialog = ref(false)
const editingTask = ref<Task | null>(null)

// The shell already loads the projects for the tree; the task cards read the
// same cache for their project names.
onMounted(async () => {
  await Promise.all([tasksStore.loadTasks(), projectsStore.load()])
})

const activeTaskId = computed<number | null>(() =>
  route.params.taskId === undefined ? null : Number(route.params.taskId),
)

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
    if (selectedFlowFilter.value && t.flowType !== selectedFlowFilter.value) {
      return false
    }
    if (selectedStatusFilter.value && t.status !== selectedStatusFilter.value) {
      return false
    }
    return true
  })
})

// The list arriving (or a task being renamed) is the tab strip's chance to
// adopt the readable task title, matching what the session titles do.
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

function openCreateDialog() {
  editingTask.value = null
  showManageDialog.value = true
}

function onSaved(task: Task) {
  showManageDialog.value = false
  selectTask(task.id)
}

function getProjectName(pid: number) {
  return projectsStore.byId(pid)?.name ?? `项目 #${pid}`
}
</script>

<template>
  <div class="tasks-sidebar" aria-label="任务列表">
    <div class="sidebar-head">
      <h2>任务</h2>
      <div class="head-actions">
        <button type="button" class="new-task" @click="openCreateDialog">新建</button>
      </div>
    </div>

    <input
      v-model="searchKeyword"
      type="search"
      class="search"
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

      <select v-model="selectedFlowFilter" class="filter-select flow-filter" aria-label="按流程筛选">
        <option :value="undefined">全部流程</option>
        <option value="transactional">事务型</option>
        <option value="development">开发类</option>
      </select>

      <select
        v-model="selectedStatusFilter"
        class="filter-select status-filter"
        aria-label="按状态筛选"
      >
        <option :value="undefined">全部状态</option>
        <option v-for="status in statusOptions" :key="status" :value="status">
          {{ getStatusLabel(status) }}
        </option>
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
        @click="selectTask(t.id)"
      >
        <div class="card-top">
          <span class="flow-pill" :class="t.flowType">
            {{ t.flowType === 'transactional' ? '事务型' : '开发类' }}
          </span>
          <span class="status-badge" :class="getStatusBadgeClass(t.status)">
            {{ getStatusLabel(t.status) }}
          </span>
        </div>

        <h4 class="card-title">{{ t.title }}</h4>

        <div class="card-meta">
          <span class="meta-project">{{ getProjectName(t.projectId) }}</span>
          <span class="meta-step">
            步骤 {{ t.currentStep }}/{{ t.flowType === 'transactional' ? 2 : 4 }}
          </span>
        </div>

        <div v-if="t.scheduleEnabled" class="schedule-tag">
          <span class="tag-icon">⏰</span>
          <span v-if="t.scheduleType === 'cron'">周期调度: {{ t.cronExpr }}</span>
          <span v-else>定时调度</span>
        </div>
      </div>
    </div>

    <TaskManageDialog v-if="showManageDialog" :task="editingTask" @close="showManageDialog = false" @saved="onSaved" />
  </div>
</template>

<style scoped>
@import './tasks-view.css';
</style>

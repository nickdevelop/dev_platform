<script setup lang="ts">
// Task workspace view: coordinates task creation, task detail display,
// and empty state guidance based on the active route.
import { computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useProjectsStore } from '../../stores/projects'
import { useTasksStore } from '../../stores/tasks'
import TaskCreatePanel from './TaskCreatePanel.vue'
import TaskDetailView from './TaskDetailView.vue'
import { UiButton, UiEmpty } from '../../shared/ui'

const route = useRoute()
const router = useRouter()
const tasksStore = useTasksStore()
const projectsStore = useProjectsStore()

onMounted(async () => {
  await Promise.all([projectsStore.load(), tasksStore.loadTasks()])
})

const isNewRoute = computed(() => route.name === 'task-new')

const activeTaskId = computed<number | null>(() => {
  if (isNewRoute.value) return null
  if (route.params.taskId !== undefined) {
    const id = Number(route.params.taskId)
    return isNaN(id) ? null : id
  }
  // When visiting /tasks directly, fall back to first task if available
  return tasksStore.tasks.length > 0 ? tasksStore.tasks[0].id : null
})

// Auto-navigate to first task if on plain /tasks route and tasks exist
watch(
  [() => route.name, () => tasksStore.tasks.length],
  ([routeName, count]) => {
    if (routeName === 'tasks' && count > 0 && !isNewRoute.value) {
      const firstId = tasksStore.tasks[0].id
      void router.replace({ name: 'task-detail', params: { taskId: firstId } })
    }
  },
  { immediate: true },
)

function onTaskCreated(taskId: number) {
  void router.push({ name: 'task-detail', params: { taskId } })
}

function onTaskDeleted() {
  if (tasksStore.tasks.length > 0) {
    void router.push({ name: 'task-detail', params: { taskId: tasksStore.tasks[0].id } })
  } else {
    void router.push({ name: 'tasks' })
  }
}

function onCancelCreate() {
  if (tasksStore.tasks.length > 0) {
    void router.push({ name: 'task-detail', params: { taskId: tasksStore.tasks[0].id } })
  } else {
    void router.push({ name: 'tasks' })
  }
}
</script>

<template>
  <main class="tasks-workbench flex flex-col h-full min-h-0 overflow-hidden bg-slate-50 dark:bg-slate-950" data-test="tasks-workbench">
    <!-- 1. Task Creation Panel -->
    <template v-if="isNewRoute">
      <TaskCreatePanel
        @created="onTaskCreated"
        @cancel="onCancelCreate"
      />
    </template>

    <!-- 2. Task Detail View -->
    <template v-else-if="activeTaskId">
      <TaskDetailView
        :task-id="activeTaskId"
        @deleted="onTaskDeleted"
      />
    </template>

    <!-- 3. Empty Workbench Guidance -->
    <template v-else>
      <div class="empty-workbench flex items-center justify-center h-full p-8" data-test="empty-workbench">
        <div class="empty-card max-w-md w-full p-8 text-center bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded shadow-sm">
          <UiEmpty
            title="任务中心"
            description="任务模块支持将 Agent 会话配置为可复用的模板，并支持手动触发、定时触发或周期性自动运行。"
            icon="⚡"
          >
            <template #actions>
              <UiButton
                variant="primary"
                class="btn-new-task-cta"
                data-test="btn-create-first-task"
                @click="router.push({ name: 'task-new' })"
              >
                + 创建第一个任务
              </UiButton>
            </template>
          </UiEmpty>
        </div>
      </div>
    </template>
  </main>
</template>

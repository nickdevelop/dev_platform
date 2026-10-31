<script setup lang="ts">
// Task workspace view: coordinates task creation, task detail display,
// and empty state guidance based on the active route.
import { computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useProjectsStore } from '../../stores/projects'
import { useTasksStore } from '../../stores/tasks'
import TaskCreatePanel from './TaskCreatePanel.vue'
import TaskDetailView from './TaskDetailView.vue'
import { UiButton } from '../../shared/ui'

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
  <main class="tasks-workbench flex flex-col h-full min-h-0 overflow-hidden" data-test="tasks-workbench">
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
      <div class="empty-workbench flex items-center justify-center h-full p-6 md:p-12 overflow-y-auto" data-test="empty-workbench">
        <div class="max-w-2xl w-full bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius,6px)] p-8 md:p-10 flex flex-col gap-8">
          <!-- Header info -->
          <div class="space-y-2">
            <div class="inline-flex items-center gap-2 text-xs font-medium text-[var(--color-text-muted)] bg-[var(--color-surface-muted)] px-2.5 py-1 rounded border border-[var(--color-border)]">
              <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
                <circle cx="8" cy="8" r="6" />
                <polyline points="8 4.5 8 8 10.5 9.5" />
              </svg>
              <span>自动化会话中枢</span>
            </div>
            <h1 class="text-xl md:text-2xl font-semibold text-[var(--color-text)] tracking-tight">
              任务中心
            </h1>
            <p class="text-sm text-[var(--color-text-muted)] leading-relaxed max-w-xl">
              任务模块支持将 Agent 会话配置为可复用的模板，支持手动触发、未来时间点定时执行或标准 Cron 周期性自动化循环运行。
            </p>
          </div>

          <!-- Feature trio -->
          <div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <div class="p-4 rounded border border-[var(--color-border)] bg-[var(--color-surface-muted)] flex flex-col gap-2">
              <span class="text-xs font-semibold text-[var(--color-text)] flex items-center gap-1.5">
                <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.6" class="text-[var(--color-primary)]">
                  <rect x="2.5" y="2.5" width="11" height="11" rx="1.5" />
                  <line x1="5.5" y1="6" x2="10.5" y2="6" />
                  <line x1="5.5" y1="9" x2="8.5" y2="9" />
                </svg>
                会话模板化
              </span>
              <p class="text-xs text-[var(--color-text-muted)] leading-relaxed">
                预设项目环境、模型与提示词规则，保障多轮执行的稳定性与一致性。
              </p>
            </div>

            <div class="p-4 rounded border border-[var(--color-border)] bg-[var(--color-surface-muted)] flex flex-col gap-2">
              <span class="text-xs font-semibold text-[var(--color-text)] flex items-center gap-1.5">
                <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.6" class="text-[var(--color-primary)]">
                  <circle cx="8" cy="8" r="6" />
                  <polyline points="8 4.5 8 8 10.5 9.5" />
                </svg>
                灵活调度策略
              </span>
              <p class="text-xs text-[var(--color-text-muted)] leading-relaxed">
                支持手动单次、指定未来时刻运行，或按 Cron 表达式定期巡检。
              </p>
            </div>

            <div class="p-4 rounded border border-[var(--color-border)] bg-[var(--color-surface-muted)] flex flex-col gap-2">
              <span class="text-xs font-semibold text-[var(--color-text)] flex items-center gap-1.5">
                <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.6" class="text-[var(--color-primary)]">
                  <path d="M2.5 4h11v7H5l-2.5 2.5V4z" />
                </svg>
                独立运行记录
              </span>
              <p class="text-xs text-[var(--color-text-muted)] leading-relaxed">
                每次执行自动生成完整对话历史与状态沉淀，随时回溯与介入操作。
              </p>
            </div>
          </div>

          <!-- Bottom Action -->
          <div class="pt-2 border-t border-[var(--color-border)] flex items-center justify-between gap-4 flex-wrap">
            <span class="text-xs text-[var(--color-text-muted)]">
              创建后即可在列表中直接执行与排期监控。
            </span>
            <UiButton
              variant="primary"
              class="btn-new-task-cta px-4 py-2 font-medium text-xs"
              data-test="btn-create-first-task"
              @click="router.push({ name: 'task-new' })"
            >
              + 创建第一个任务
            </UiButton>
          </div>
        </div>
      </div>
    </template>
  </main>
</template>

<style scoped>
@import './tasks-view.css';
</style>

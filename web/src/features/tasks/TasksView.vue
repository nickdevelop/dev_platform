<script setup lang="ts">
// Task workspace view: coordinates task creation, task detail display,
// and empty state guidance based on the active route.
import { computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useProjectsStore } from '../../stores/projects'
import { useTasksStore } from '../../stores/tasks'
import TaskCreatePanel from './TaskCreatePanel.vue'
import TaskDetailView from './TaskDetailView.vue'

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
  <main class="tasks-workbench" data-test="tasks-workbench">
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
      <div class="empty-workbench" data-test="empty-workbench">
        <div class="empty-card">
          <div class="empty-icon">⚡</div>
          <h3>任务中心</h3>
          <p class="empty-desc">
            任务模块支持将 Agent 会话配置为可复用的模板，并支持手动触发、定时触发或周期性自动运行。
          </p>
          <button
            type="button"
            class="btn-new-task-cta"
            data-test="btn-create-first-task"
            @click="router.push({ name: 'task-new' })"
          >
            + 创建第一个任务
          </button>
        </div>
      </div>
    </template>
  </main>
</template>

<style scoped>
.tasks-workbench {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  overflow: hidden;
  background: var(--color-bg);
}

.empty-workbench {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  padding: 2rem;
}

.empty-card {
  text-align: center;
  max-width: 28rem;
  padding: 2.5rem 2rem;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 12px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.05);
}

.empty-icon {
  font-size: 2.5rem;
  margin-bottom: 0.75rem;
}

.empty-card h3 {
  font-size: 1.25rem;
  font-weight: 700;
  color: var(--color-text);
  margin: 0 0 0.5rem 0;
}

.empty-desc {
  font-size: 0.9rem;
  color: var(--color-text-muted);
  line-height: 1.5;
  margin: 0 0 1.5rem 0;
}

.btn-new-task-cta {
  padding: 0.6rem 1.4rem;
  background: var(--color-primary, #3b82f6);
  color: #fff;
  border: none;
  border-radius: 6px;
  font-weight: 600;
  font-size: 0.95rem;
  cursor: pointer;
  transition: opacity 0.15s;
}

.btn-new-task-cta:hover {
  opacity: 0.9;
}
</style>

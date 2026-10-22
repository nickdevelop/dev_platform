// Store for Task Management and Execution.
import { defineStore } from 'pinia'
import { describeApiError, UnauthorizedError } from '../api/client'
import {
  createTask,
  deleteTask,
  getTask,
  listTaskRuns,
  listTasks,
  triggerTaskExecution,
  updateTask,
  type CreateTaskPayload,
  type Task,
  type TaskRun,
  type TaskStatus,
  type TaskType,
  type UpdateTaskPayload,
} from '../api/tasks'
import { useAuthStore } from './auth'
import { tabKey, useOpenTabsStore } from './openTabs'

export const useTasksStore = defineStore('tasks', {
  state: () => ({
    tasks: [] as Task[],
    currentTask: null as Task | null,
    taskRuns: {} as Record<number, TaskRun[]>,
    loading: false,
    error: '',
  }),

  getters: {
    byId: (state) => (id: number): Task | undefined =>
      state.tasks.find((task) => task.id === id),
    runsByTaskId: (state) => (taskId: number): TaskRun[] =>
      state.taskRuns[taskId] || [],
  },

  actions: {
    async loadTasks(filter?: {
      projectId?: number
      taskType?: TaskType
      status?: TaskStatus
      keyword?: string
    }) {
      this.loading = true
      try {
        this.tasks = await listTasks(filter)
        this.error = ''
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
      } finally {
        this.loading = false
      }
    },

    async loadTask(id: number) {
      this.loading = true
      try {
        const task = await getTask(id)
        this.currentTask = task
        const idx = this.tasks.findIndex((t) => t.id === id)
        if (idx !== -1) {
          this.tasks[idx] = task
        } else {
          this.tasks.unshift(task)
        }
        this.error = ''
        return task
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
        throw error
      } finally {
        this.loading = false
      }
    },

    async createTask(payload: CreateTaskPayload) {
      const auth = useAuthStore()
      const task = await createTask(payload)
      this.tasks.unshift(task)
      this.currentTask = task
      return task
    },

    async updateTask(id: number, payload: UpdateTaskPayload) {
      const auth = useAuthStore()
      const task = await updateTask(id, payload)
      const idx = this.tasks.findIndex((t) => t.id === id)
      if (idx !== -1) {
        this.tasks[idx] = task
      }
      if (this.currentTask?.id === id) {
        this.currentTask = task
      }
      return task
    },

    async deleteTask(id: number) {
      const auth = useAuthStore()
      await deleteTask(id)
      this.tasks = this.tasks.filter((t) => t.id !== id)
      delete this.taskRuns[id]
      if (this.currentTask?.id === id) {
        this.currentTask = null
      }
      const openTabs = useOpenTabsStore()
      openTabs.closeTab(`task:${id}`)
    },

    async triggerExecution(id: number, triggerType: 'manual' | 'once' | 'cron' = 'manual') {
      const auth = useAuthStore()
      const res = await triggerTaskExecution(id, triggerType)
      if (res.run) {
        const existingRuns = this.taskRuns[id] || []
        this.taskRuns[id] = [res.run, ...existingRuns]
      }
      // Reload task to sync status
      await this.loadTask(id)
      return res
    },

    async loadTaskRuns(taskId: number) {
      try {
        const runs = await listTaskRuns(taskId)
        this.taskRuns[taskId] = runs
        return runs
      } catch (error) {
        if (error instanceof UnauthorizedError) return []
        console.error('Failed to load task runs', error)
        return []
      }
    },
  },
})

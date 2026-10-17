// Store for Task Management and Orchestration Flows.
import { defineStore } from 'pinia'
import { describeApiError, UnauthorizedError } from '../api/client'
import {
  advanceTaskStep,
  createTask,
  createTaskSession,
  deleteTask,
  getFlowDefs,
  getTask,
  listTasks,
  triggerTaskExecution,
  updateTask,
  updateTaskSchedule,
  type AdvanceStepPayload,
  type CreateTaskPayload,
  type FlowDef,
  type FlowType,
  type ScheduleConfigPayload,
  type Task,
  type TaskStatus,
  type UpdateTaskPayload,
} from '../api/tasks'
import { useAuthStore } from './auth'

export const useTasksStore = defineStore('tasks', {
  state: () => ({
    tasks: [] as Task[],
    currentTask: null as Task | null,
    flowDefs: [] as FlowDef[],
    loading: false,
    error: '',
  }),

  getters: {
    byId: (state) => (id: number): Task | undefined =>
      state.tasks.find((task) => task.id === id),
    flowDefByType: (state) => (type: FlowType): FlowDef | undefined =>
      state.flowDefs.find((f) => f.type === type),
  },

  actions: {
    async loadFlowDefs() {
      if (this.flowDefs.length > 0) return
      try {
        this.flowDefs = await getFlowDefs()
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        console.error('Failed to load flow defs', error)
      }
    },

    async loadTasks(filter?: {
      projectId?: number
      flowType?: FlowType
      status?: TaskStatus
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
        // Update in list if present
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
      const task = await createTask(auth.csrfToken, payload)
      this.tasks.unshift(task)
      this.currentTask = task
      return task
    },

    async updateTask(id: number, payload: UpdateTaskPayload) {
      const auth = useAuthStore()
      const task = await updateTask(auth.csrfToken, id, payload)
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
      await deleteTask(auth.csrfToken, id)
      this.tasks = this.tasks.filter((t) => t.id !== id)
      if (this.currentTask?.id === id) {
        this.currentTask = null
      }
    },

    async advanceStep(id: number, payload: AdvanceStepPayload) {
      const auth = useAuthStore()
      const task = await advanceTaskStep(auth.csrfToken, id, payload)
      const idx = this.tasks.findIndex((t) => t.id === id)
      if (idx !== -1) {
        this.tasks[idx] = task
      }
      if (this.currentTask?.id === id) {
        this.currentTask = task
      }
      return task
    },

    async updateSchedule(id: number, payload: ScheduleConfigPayload) {
      const auth = useAuthStore()
      const task = await updateTaskSchedule(auth.csrfToken, id, payload)
      const idx = this.tasks.findIndex((t) => t.id === id)
      if (idx !== -1) {
        this.tasks[idx] = task
      }
      if (this.currentTask?.id === id) {
        this.currentTask = task
      }
      return task
    },

    async createSession(id: number) {
      const auth = useAuthStore()
      const res = await createTaskSession(auth.csrfToken, id)
      const idx = this.tasks.findIndex((t) => t.id === id)
      if (idx !== -1) {
        this.tasks[idx] = res.task
      }
      if (this.currentTask?.id === id) {
        this.currentTask = res.task
      }
      return res
    },

    async triggerExecution(id: number) {
      const auth = useAuthStore()
      const res = await triggerTaskExecution(auth.csrfToken, id)
      const idx = this.tasks.findIndex((t) => t.id === id)
      if (idx !== -1) {
        this.tasks[idx] = res.task
      }
      if (this.currentTask?.id === id) {
        this.currentTask = res.task
      }
      return res
    },
  },
})

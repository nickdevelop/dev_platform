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
  listTaskRoles,
  createTaskRole,
  updateTaskRole,
  deleteTaskRole,
  listTaskFlowRuns,
  getFlowRun,
  listFlowMessages,
  postFlowMessage,
  cancelFlowRun,
  type CreateTaskPayload,
  type Task,
  type TaskRun,
  type TaskStatus,
  type TaskType,
  type UpdateTaskPayload,
  type FlowRole,
  type FlowRoleInput,
  type FlowRun,
  type FlowMessage,
} from '../api/tasks'
import { tabKey, useOpenTabsStore } from './openTabs'

const activeFlowEventSources = new Map<number, EventSource>()

export const useTasksStore = defineStore('tasks', {
  state: () => ({
    tasks: [] as Task[],
    currentTask: null as Task | null,
    taskRuns: {} as Record<number, TaskRun[]>,
    taskRoles: {} as Record<number, FlowRole[]>,
    flowRuns: {} as Record<number, FlowRun[]>,
    activeFlowRun: null as FlowRun | null,
    flowMessages: {} as Record<number, FlowMessage[]>,
    loading: false,
    error: '',
  }),

  getters: {
    byId: (state) => (id: number): Task | undefined =>
      state.tasks.find((task) => task.id === id),
    runsByTaskId: (state) => (taskId: number): TaskRun[] =>
      state.taskRuns[taskId] || [],
    rolesByTaskId: (state) => (taskId: number): FlowRole[] =>
      state.taskRoles[taskId] || [],
    flowRunsByTaskId: (state) => (taskId: number): FlowRun[] =>
      state.flowRuns[taskId] || [],
    messagesByRunId: (state) => (runId: number): FlowMessage[] =>
      state.flowMessages[runId] || [],
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
      const task = await createTask(payload)
      this.tasks.unshift(task)
      this.currentTask = task
      return task
    },

    async updateTask(id: number, payload: UpdateTaskPayload) {
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

    async loadRoles(taskId: number) {
      try {
        const roles = await listTaskRoles(taskId)
        this.taskRoles[taskId] = roles
        return roles
      } catch (error) {
        if (error instanceof UnauthorizedError) return []
        console.error('Failed to load task roles', error)
        return []
      }
    },

    async createRole(taskId: number, payload: FlowRoleInput) {
      const created = await createTaskRole(taskId, payload)
      if (!this.taskRoles[taskId]) {
        this.taskRoles[taskId] = []
      }
      this.taskRoles[taskId].push(created)
      return created
    },

    async updateRole(taskId: number, roleId: number, payload: FlowRoleInput) {
      const updated = await updateTaskRole(taskId, roleId, payload)
      if (this.taskRoles[taskId]) {
        const idx = this.taskRoles[taskId].findIndex((r) => r.id === roleId)
        if (idx !== -1) {
          this.taskRoles[taskId][idx] = updated
        }
      }
      return updated
    },

    async deleteRole(taskId: number, roleId: number) {
      await deleteTaskRole(taskId, roleId)
      if (this.taskRoles[taskId]) {
        this.taskRoles[taskId] = this.taskRoles[taskId].filter((r) => r.id !== roleId)
      }
    },

    async loadFlowRuns(taskId: number) {
      try {
        const runs = await listTaskFlowRuns(taskId)
        this.flowRuns[taskId] = runs
        return runs
      } catch (error) {
        if (error instanceof UnauthorizedError) return []
        console.error('Failed to load flow runs', error)
        return []
      }
    },

    async loadFlowRun(runId: number) {
      try {
        const run = await getFlowRun(runId)
        this.activeFlowRun = run
        return run
      } catch (error) {
        if (error instanceof UnauthorizedError) return null
        console.error('Failed to load flow run', error)
        return null
      }
    },

    async loadFlowMessages(runId: number) {
      try {
        const messages = await listFlowMessages(runId)
        this.flowMessages[runId] = messages
        return messages
      } catch (error) {
        if (error instanceof UnauthorizedError) return []
        console.error('Failed to load flow messages', error)
        return []
      }
    },

    async postFlowMessage(runId: number, content: string) {
      const msg = await postFlowMessage(runId, content)
      if (!this.flowMessages[runId]) {
        this.flowMessages[runId] = []
      }
      // If message not yet appended by SSE, push it
      if (!this.flowMessages[runId].some((m) => m.id === msg.id)) {
        this.flowMessages[runId].push(msg)
      }
      return msg
    },

    async cancelFlowRun(runId: number) {
      const res = await cancelFlowRun(runId)
      if (this.activeFlowRun && this.activeFlowRun.id === runId) {
        this.activeFlowRun.status = 'cancelled'
      }
      return res
    },

    connectFlowEvents(runId: number) {
      this.disconnectFlowEvents(runId)
      const es = new EventSource(`/api/tasks/flows/${runId}/events`)
      activeFlowEventSources.set(runId, es)

      es.addEventListener('chunk', (e: MessageEvent) => {
        try {
          const payload = JSON.parse(e.data) as {
            messageId: number
            senderName: string
            senderType: 'user' | 'orchestrator' | 'agent' | 'system'
            chunk: string
          }
          if (!this.flowMessages[runId]) {
            this.flowMessages[runId] = []
          }
          const list = this.flowMessages[runId]
          const existing = list.find((m) => m.id === payload.messageId)
          if (existing) {
            existing.content += payload.chunk
          } else {
            list.push({
              id: payload.messageId,
              runId,
              senderType: payload.senderType,
              senderName: payload.senderName,
              content: payload.chunk,
              status: 'streaming',
              createdAt: new Date().toISOString(),
              updatedAt: new Date().toISOString(),
            })
          }
        } catch (err) {
          console.error('Failed to parse flow chunk event', err)
        }
      })

      es.addEventListener('message', (e: MessageEvent) => {
        try {
          const msg = JSON.parse(e.data) as FlowMessage
          if (!this.flowMessages[runId]) {
            this.flowMessages[runId] = []
          }
          const list = this.flowMessages[runId]
          const idx = list.findIndex((m) => m.id === msg.id)
          if (idx !== -1) {
            list[idx] = msg
          } else {
            list.push(msg)
          }
        } catch (err) {
          console.error('Failed to parse flow message event', err)
        }
      })

      es.addEventListener('status', (e: MessageEvent) => {
        try {
          const payload = JSON.parse(e.data) as {
            status: FlowRun['status']
            currentStep: number
            error?: string
          }
          if (this.activeFlowRun && this.activeFlowRun.id === runId) {
            this.activeFlowRun.status = payload.status
            this.activeFlowRun.currentStep = payload.currentStep
            if (payload.error) this.activeFlowRun.error = payload.error
          }
          for (const taskId in this.flowRuns) {
            const r = this.flowRuns[taskId]?.find((item) => item.id === runId)
            if (r) {
              r.status = payload.status
              r.currentStep = payload.currentStep
              if (payload.error) r.error = payload.error
            }
          }
        } catch (err) {
          console.error('Failed to parse flow status event', err)
        }
      })

      es.onerror = () => {
        // SSE error / auto reconnect
      }
    },

    disconnectFlowEvents(runId: number) {
      const es = activeFlowEventSources.get(runId)
      if (es) {
        es.close()
        activeFlowEventSources.delete(runId)
      }
    },
  },
})

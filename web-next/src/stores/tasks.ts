import { create } from 'zustand'
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
  type UpdateTaskPayload,
} from '../api/tasks'
import { describeApiError, UnauthorizedError } from '../api/client'

interface TasksState {
  tasks: Task[]
  runs: Record<number, TaskRun[]>
  loading: boolean
  error: string
  loadTasks: (filter?: { projectId?: number; keyword?: string }) => Promise<void>
  loadTask: (id: number) => Promise<Task | null>
  loadRuns: (id: number) => Promise<void>
  create: (payload: CreateTaskPayload) => Promise<Task | null>
  update: (id: number, payload: UpdateTaskPayload) => Promise<Task | null>
  remove: (id: number) => Promise<boolean>
  trigger: (id: number, type?: 'manual' | 'once' | 'cron') => Promise<TaskRun | null>
}

export const useTasksStore = create<TasksState>((set, getState) => ({
  tasks: [],
  runs: {},
  loading: false,
  error: '',

  loadTasks: async (filter) => {
    set({ loading: true })
    try {
      const tasks = await listTasks(filter)
      set({ tasks, loading: false, error: '' })
    } catch (error) {
      if (error instanceof UnauthorizedError) {
        set({ loading: false })
        return
      }
      set({ loading: false, error: describeApiError(error) })
    }
  },

  loadTask: async (id) => {
    const found = getState().tasks.find((task) => task.id === id)
    if (found) return found
    try {
      const task = await getTask(id)
      set((state) => ({ tasks: [task, ...state.tasks.filter((entry) => entry.id !== id)] }))
      return task
    } catch (error) {
      if (error instanceof UnauthorizedError) return null
      set({ error: describeApiError(error) })
      return null
    }
  },

  loadRuns: async (id) => {
    try {
      const runs = await listTaskRuns(id)
      set((state) => ({ runs: { ...state.runs, [id]: runs } }))
    } catch (error) {
      if (error instanceof UnauthorizedError) return
      set({ error: describeApiError(error) })
    }
  },

  create: async (payload) => {
    try {
      const task = await createTask(payload)
      set((state) => ({ tasks: [task, ...state.tasks], error: '' }))
      return task
    } catch (error) {
      if (error instanceof UnauthorizedError) return null
      set({ error: describeApiError(error) })
      return null
    }
  },

  update: async (id, payload) => {
    try {
      const task = await updateTask(id, payload)
      set((state) => ({ tasks: state.tasks.map((entry) => (entry.id === id ? task : entry)) }))
      return task
    } catch (error) {
      if (error instanceof UnauthorizedError) return null
      set({ error: describeApiError(error) })
      return null
    }
  },

  remove: async (id) => {
    try {
      await deleteTask(id)
      set((state) => ({ tasks: state.tasks.filter((task) => task.id !== id) }))
      return true
    } catch (error) {
      if (error instanceof UnauthorizedError) return false
      set({ error: describeApiError(error) })
      return false
    }
  },

  trigger: async (id, type = 'manual') => {
    try {
      const result = await triggerTaskExecution(id, type)
      await getState().loadRuns(id)
      return result.run
    } catch (error) {
      if (error instanceof UnauthorizedError) return null
      set({ error: describeApiError(error) })
      return null
    }
  },
}))

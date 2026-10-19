// API client for Task Management and Execution.
import { apiRequest } from './client'
import type { BackendId } from './sessions'

export type TaskType = 'standard' | 'flow'
export type ScheduleType = 'manual' | 'once' | 'cron'
export type RunStatus = 'idle' | 'running' | 'success' | 'failed'
export type TaskStatus = 'active' | 'disabled'

export interface Task {
  id: number
  title: string
  description: string
  taskType: TaskType
  projectId: number
  backend: BackendId
  model: string
  promptTemplate: string
  scheduleType: ScheduleType
  scheduledAt?: string
  cronExpr?: string
  scheduleEnabled: boolean
  lastRunAt?: string
  nextRunAt?: string
  lastRunStatus?: RunStatus
  status: TaskStatus
  createdAt: string
  updatedAt: string
}

export interface TaskRun {
  id: number
  taskId: number
  sessionId: string
  triggerType: 'manual' | 'once' | 'cron'
  status: 'running' | 'success' | 'failed'
  prompt: string
  error?: string
  triggeredAt: string
  finishedAt?: string
  createdAt: string
}

export interface CreateTaskPayload {
  title: string
  description?: string
  taskType?: TaskType
  projectId: number
  backend: BackendId
  model: string
  promptTemplate: string
  scheduleType?: ScheduleType
  scheduledAt?: string
  cronExpr?: string
  scheduleEnabled?: boolean
}

export interface UpdateTaskPayload {
  title?: string
  description?: string
  projectId?: number
  backend?: BackendId
  model?: string
  promptTemplate?: string
  scheduleType?: ScheduleType
  scheduledAt?: string
  cronExpr?: string
  scheduleEnabled?: boolean
  status?: TaskStatus
}

export async function listTasks(filter?: {
  projectId?: number
  taskType?: TaskType
  status?: TaskStatus
  keyword?: string
}): Promise<Task[]> {
  const params = new URLSearchParams()
  if (filter?.projectId) params.set('projectId', String(filter.projectId))
  if (filter?.taskType) params.set('taskType', filter.taskType)
  if (filter?.status) params.set('status', filter.status)
  if (filter?.keyword) params.set('keyword', filter.keyword)

  const query = params.toString() ? `?${params.toString()}` : ''
  const res = await apiRequest<{ tasks: Task[] }>('GET', `/tasks${query}`)
  return res.tasks
}

export async function getTask(id: number): Promise<Task> {
  return apiRequest<Task>('GET', `/tasks/${id}`)
}

export async function createTask(csrfToken: string, payload: CreateTaskPayload): Promise<Task> {
  return apiRequest<Task>('POST', '/tasks', payload, { csrfToken })
}

export async function updateTask(
  csrfToken: string,
  id: number,
  payload: UpdateTaskPayload,
): Promise<Task> {
  return apiRequest<Task>('PUT', `/tasks/${id}`, payload, { csrfToken })
}

export async function deleteTask(csrfToken: string, id: number): Promise<void> {
  await apiRequest<void>('DELETE', `/tasks/${id}`, undefined, { csrfToken })
}

export async function triggerTaskExecution(
  csrfToken: string,
  id: number,
  triggerType: 'manual' | 'once' | 'cron' = 'manual',
): Promise<{ run: TaskRun }> {
  return apiRequest<{ run: TaskRun }>('POST', `/tasks/${id}/trigger`, { triggerType }, { csrfToken })
}

export async function listTaskRuns(id: number): Promise<TaskRun[]> {
  const res = await apiRequest<{ runs: TaskRun[] }>('GET', `/tasks/${id}/runs`)
  return res.runs
}

export async function listTaskSessionIds(projectId?: number): Promise<string[]> {
  const query = projectId ? `?projectId=${projectId}` : ''
  const res = await apiRequest<{ sessionIds: string[] }>('GET', `/tasks/session-ids${query}`)
  return res.sessionIds
}

export const getTaskSessionIds = listTaskSessionIds

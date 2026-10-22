// API client for Task Management and Execution.
import { get, mutate } from './client'
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
  const res = await get<{ tasks: Task[] }>(`/tasks${query}`)
  return res.tasks
}

export async function getTask(id: number): Promise<Task> {
  return get<Task>(`/tasks/${id}`)
}

export async function createTask(payload: CreateTaskPayload): Promise<Task> {
  return mutate<Task>('POST', '/tasks', payload)
}

export async function updateTask(id: number, payload: UpdateTaskPayload): Promise<Task> {
  return mutate<Task>('PUT', `/tasks/${id}`, payload)
}

export async function deleteTask(id: number): Promise<void> {
  await mutate<void>('DELETE', `/tasks/${id}`)
}

export async function triggerTaskExecution(
  id: number,
  triggerType: 'manual' | 'once' | 'cron' = 'manual',
): Promise<{ run: TaskRun }> {
  return mutate<{ run: TaskRun }>('POST', `/tasks/${id}/trigger`, { triggerType })
}

export async function listTaskRuns(id: number): Promise<TaskRun[]> {
  const res = await get<{ runs: TaskRun[] }>(`/tasks/${id}/runs`)
  return res.runs
}

export async function listTaskSessionIds(projectId?: number): Promise<string[]> {
  const query = projectId ? `?projectId=${projectId}` : ''
  const res = await get<{ sessionIds: string[] }>(`/tasks/session-ids${query}`)
  return res.sessionIds
}

export const getTaskSessionIds = listTaskSessionIds

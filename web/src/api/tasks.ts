// API client for Task Management and Orchestration Flows.
import { apiRequest } from './client'
import type { BackendId } from './sessions'

export type FlowType = 'transactional' | 'development'
export type StepStatus = 'pending' | 'in_progress' | 'confirmed' | 'completed'
export type TaskStatus =
  | 'draft'
  | 'in_progress'
  | 'ready_for_execution'
  | 'executing'
  | 'completed'
  | 'failed'

export type ScheduleType = 'none' | 'once' | 'cron'
export type RunStatus = 'idle' | 'running' | 'success' | 'failed' | 'skipped_not_ready'

export interface StepDef {
  index: number
  key: string
  name: string
  description: string
  prompt: string
  isFinal: boolean
}

export interface FlowDef {
  type: FlowType
  name: string
  description: string
  steps: StepDef[]
}

export interface Task {
  id: number
  title: string
  description: string
  projectId: number
  backend: BackendId
  model: string
  flowType: FlowType
  currentStep: number
  stepStatus: StepStatus
  status: TaskStatus
  isConfirmed: boolean
  stepsData: string
  activeSessionId: string
  scheduleType: ScheduleType
  scheduledAt?: string
  cronExpr?: string
  scheduleEnabled: boolean
  lastRunAt?: string
  nextRunAt?: string
  lastRunStatus?: RunStatus
  executionPrompt?: string
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
  projectId: number
  backend: BackendId
  model: string
  flowType: FlowType
}

export interface UpdateTaskPayload {
  title?: string
  description?: string
  projectId?: number
  backend?: BackendId
  model?: string
}

export interface AdvanceStepPayload {
  confirmed: boolean
  nextStep?: number
  notes?: string
  driverPrompt?: string
  executionPrompt?: string
}

export interface ScheduleConfigPayload {
  scheduleType: ScheduleType
  scheduledAt?: string
  cronExpr?: string
  scheduleEnabled: boolean
}

export interface TaskSessionResponse {
  task: Task
  session: {
    id: string
    projectId: number
    backend: BackendId
    title: string
    status: string
  }
}

export async function getFlowDefs(): Promise<FlowDef[]> {
  const res = await apiRequest<{ flows: FlowDef[] }>('GET', '/tasks/flows')
  return res.flows
}

export async function listTasks(filter?: {
  projectId?: number
  flowType?: FlowType
  status?: TaskStatus
}): Promise<Task[]> {
  const params = new URLSearchParams()
  if (filter?.projectId) params.set('projectId', String(filter.projectId))
  if (filter?.flowType) params.set('flowType', filter.flowType)
  if (filter?.status) params.set('status', filter.status)

  const qs = params.toString()
  const path = qs ? `/tasks?${qs}` : '/tasks'
  const res = await apiRequest<{ tasks: Task[] }>('GET', path)
  return res.tasks
}

export async function getTask(id: number): Promise<Task> {
  return apiRequest<Task>('GET', `/tasks/${id}`)
}

export async function createTask(csrfToken: string, payload: CreateTaskPayload): Promise<Task> {
  return apiRequest<Task>('POST', '/tasks', {
    csrfToken,
    body: payload,
  })
}

export async function updateTask(
  csrfToken: string,
  id: number,
  payload: UpdateTaskPayload,
): Promise<Task> {
  return apiRequest<Task>('PUT', `/tasks/${id}`, {
    csrfToken,
    body: payload,
  })
}

export async function deleteTask(csrfToken: string, id: number): Promise<void> {
  await apiRequest<void>('DELETE', `/tasks/${id}`, {
    csrfToken,
  })
}

export async function advanceTaskStep(
  csrfToken: string,
  id: number,
  payload: AdvanceStepPayload,
): Promise<Task> {
  return apiRequest<Task>('POST', `/tasks/${id}/advance`, {
    csrfToken,
    body: payload,
  })
}

export async function updateTaskSchedule(
  csrfToken: string,
  id: number,
  payload: ScheduleConfigPayload,
): Promise<Task> {
  return apiRequest<Task>('PUT', `/tasks/${id}/schedule`, {
    csrfToken,
    body: payload,
  })
}

export async function createTaskSession(csrfToken: string, id: number): Promise<TaskSessionResponse> {
  return apiRequest<TaskSessionResponse>('POST', `/tasks/${id}/session`, {
    csrfToken,
  })
}

export async function saveExecutionPrompt(
  csrfToken: string,
  id: number,
  executionPrompt: string,
): Promise<Task> {
  return apiRequest<Task>('PUT', `/tasks/${id}/execution-prompt`, {
    csrfToken,
    body: { executionPrompt },
  })
}

export async function listTaskRuns(id: number): Promise<TaskRun[]> {
  const res = await apiRequest<{ runs: TaskRun[] }>('GET', `/tasks/${id}/runs`)
  return res.runs
}

export async function listTaskSessionIds(projectId?: number): Promise<string[]> {
  const qs = projectId ? `?projectId=${projectId}` : ''
  const res = await apiRequest<{ sessionIds: string[] }>('GET', `/tasks/session-ids${qs}`)
  return res.sessionIds || []
}

export async function triggerTaskExecution(
  csrfToken: string,
  id: number,
  triggerType?: string,
): Promise<{ message: string; task: Task; run: TaskRun }> {
  const qs = triggerType ? `?triggerType=${encodeURIComponent(triggerType)}` : ''
  return apiRequest<{ message: string; task: Task; run: TaskRun }>('POST', `/tasks/${id}/trigger${qs}`, {
    csrfToken,
  })
}

// API client for Team Groups and Group Tasks.
import { get, mutate } from './client'
import type { AgentSession } from './sessions'
import type { FlowMessage, FlowRun } from './tasks'

export interface Group {
  id: number
  name: string
  description: string
  orchestratorModel: string
  orchestratorPrompt: string
  icon?: string
  color?: string
  memberCount: number
  taskCount: number
  createdAt: string
  updatedAt: string
}

export interface GroupMemberDetail {
  id: number
  groupId: number
  roleId: number
  sortOrder: number
  name: string
  description: string
  backend: string
  model: string
  promptId: number
  systemPrompt: string
  icon?: string
  color?: string
  createdAt: string
}

export interface GroupDetail extends Group {
  members: GroupMemberDetail[]
}

export interface CreateGroupPayload {
  name: string
  description?: string
  orchestratorModel: string
  orchestratorPrompt?: string
  icon?: string
  color?: string
  roleIds?: number[]
}

export interface UpdateGroupPayload {
  name?: string
  description?: string
  orchestratorModel?: string
  orchestratorPrompt?: string
  icon?: string
  color?: string
}

export interface GroupTask {
  id: number
  groupId: number
  title: string
  description: string
  projectId: number
  worktreeId: number
  prompt: string
  scheduleType: 'manual' | 'once' | 'cron'
  scheduledAt?: string
  cronExpr?: string
  scheduleEnabled: boolean
  lastRunAt?: string
  nextRunAt?: string
  lastRunStatus?: 'idle' | 'running' | 'success' | 'failed'
  status: 'active' | 'disabled'
  createdAt: string
  updatedAt: string
}

export interface CreateGroupTaskPayload {
  title: string
  description?: string
  projectId: number
  worktreeId?: number
  prompt: string
  scheduleType?: 'manual' | 'once' | 'cron'
  scheduledAt?: string
  cronExpr?: string
  scheduleEnabled?: boolean
}

export interface UpdateGroupTaskPayload {
  title?: string
  description?: string
  projectId?: number
  worktreeId?: number
  prompt?: string
  scheduleType?: 'manual' | 'once' | 'cron'
  scheduledAt?: string
  cronExpr?: string
  scheduleEnabled?: boolean
  status?: 'active' | 'disabled'
}

interface GroupListResponse {
  groups: Group[]
}

interface GroupTasksResponse {
  tasks: GroupTask[]
}

interface GroupRunsResponse {
  runs: FlowRun[]
}

// Group Operations
export async function listGroups(): Promise<Group[]> {
  const res = await get<GroupListResponse>('/groups')
  return res.groups
}

export async function getGroup(id: number): Promise<GroupDetail> {
  return get<GroupDetail>(`/groups/${id}`)
}

export async function createGroup(payload: CreateGroupPayload): Promise<GroupDetail> {
  return mutate<GroupDetail>('POST', '/groups', payload)
}

export async function updateGroup(id: number, payload: UpdateGroupPayload): Promise<Group> {
  return mutate<Group>('PUT', `/groups/${id}`, payload)
}

export async function deleteGroup(id: number): Promise<void> {
  await mutate<void>('DELETE', `/groups/${id}`)
}

export async function setGroupMembers(groupId: number, roleIds: number[]): Promise<GroupMemberDetail[]> {
  const res = await mutate<{ members: GroupMemberDetail[] }>('PUT', `/groups/${groupId}/members`, { roleIds })
  return res.members
}

// Group Task Operations
export async function listGroupTasks(groupId: number): Promise<GroupTask[]> {
  const res = await get<GroupTasksResponse>(`/groups/${groupId}/tasks`)
  return res.tasks
}

export async function getGroupTask(groupId: number, taskId: number): Promise<GroupTask> {
  return get<GroupTask>(`/groups/${groupId}/tasks/${taskId}`)
}

export async function createGroupTask(groupId: number, payload: CreateGroupTaskPayload): Promise<GroupTask> {
  return mutate<GroupTask>('POST', `/groups/${groupId}/tasks`, payload)
}

export async function updateGroupTask(
  groupId: number,
  taskId: number,
  payload: UpdateGroupTaskPayload,
): Promise<GroupTask> {
  return mutate<GroupTask>('PUT', `/groups/${groupId}/tasks/${taskId}`, payload)
}

export async function deleteGroupTask(groupId: number, taskId: number): Promise<void> {
  await mutate<void>('DELETE', `/groups/${groupId}/tasks/${taskId}`)
}

export async function listGroupTaskRuns(groupId: number, taskId: number): Promise<FlowRun[]> {
  const res = await get<GroupRunsResponse>(`/groups/${groupId}/tasks/${taskId}/runs`)
  return res.runs
}

export async function listGroupRunMessages(runId: number): Promise<FlowMessage[]> {
  const res = await get<{ messages: FlowMessage[] }>(`/groups/runs/${runId}/messages`)
  return res.messages
}

export async function triggerGroupTask(groupId: number, taskId: number, prompt?: string): Promise<FlowRun> {
  return mutate<FlowRun>('POST', `/groups/${groupId}/tasks/${taskId}/trigger`, { prompt: prompt || '' })
}

export async function postGroupRunMessage(runId: number, content: string): Promise<FlowMessage> {
  return mutate<FlowMessage>('POST', `/groups/runs/${runId}/messages`, { content })
}

export async function cancelGroupRun(runId: number): Promise<void> {
  await mutate<void>('POST', `/groups/runs/${runId}/cancel`)
}

export interface CreateCollabSessionPayload {
  projectId: number
  worktreeId?: number
  title?: string
  prompt: string
}

export interface CollabSessionResponse {
  session: AgentSession
  task: GroupTask
  run: FlowRun
}

export interface CollabSessionContext {
  session: AgentSession
  group: GroupDetail
  task: GroupTask
  runs: FlowRun[]
  activeRun?: FlowRun | null
}

export async function createCollabSession(
  groupId: number,
  payload: CreateCollabSessionPayload,
): Promise<CollabSessionResponse> {
  return mutate<CollabSessionResponse>('POST', `/groups/${groupId}/collab-sessions`, payload)
}

export async function getCollabSessionContext(sessionId: string): Promise<CollabSessionContext> {
  return get<CollabSessionContext>(`/groups/collab-sessions/${sessionId}`)
}

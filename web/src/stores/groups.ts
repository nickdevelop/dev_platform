// Pinia store for Team Groups and Group Tasks.
import { defineStore } from 'pinia'
import { describeApiError, UnauthorizedError } from '../api/client'
import {
  listGroups,
  getGroup,
  createGroup,
  updateGroup,
  deleteGroup,
  setGroupMembers,
  listGroupTasks,
  getGroupTask,
  createGroupTask,
  updateGroupTask,
  deleteGroupTask,
  listGroupTaskRuns,
  listGroupRunMessages,
  triggerGroupTask,
  postGroupRunMessage,
  cancelGroupRun,
  type Group,
  type GroupDetail,
  type GroupTask,
  type CreateGroupPayload,
  type UpdateGroupPayload,
  type CreateGroupTaskPayload,
  type UpdateGroupTaskPayload,
} from '../api/groups'
import type { FlowMessage, FlowRun } from '../api/tasks'
import { unifiedEventBus } from '../shared/events/UnifiedEventBusClient'

export const useGroupsStore = defineStore('groups', {
  state: () => ({
    groups: [] as Group[],
    activeGroupId: null as number | null,
    activeGroupDetail: null as GroupDetail | null,
    groupTasks: [] as GroupTask[],
    activeTaskId: null as number | null,
    activeTaskRuns: [] as FlowRun[],
    activeRun: null as FlowRun | null,
    flowMessages: [] as FlowMessage[],
    loading: false,
    tasksLoading: false,
    error: '',
    taskError: '',
    unsubEvents: null as (() => void) | null,
  }),

  getters: {
    groupById: (state) => (id: number): Group | undefined =>
      state.groups.find((g) => g.id === id),
    taskById: (state) => (id: number): GroupTask | undefined =>
      state.groupTasks.find((t) => t.id === id),
  },

  actions: {
    async loadGroups() {
      this.loading = true
      try {
        this.groups = await listGroups()
        this.error = ''
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
      } finally {
        this.loading = false
      }
    },

    async loadGroup(id: number) {
      this.loading = true
      try {
        const detail = await getGroup(id)
        this.activeGroupId = id
        this.activeGroupDetail = detail

        const idx = this.groups.findIndex((g) => g.id === id)
        if (idx !== -1) {
          this.groups[idx] = detail
        } else {
          this.groups.unshift(detail)
        }
        this.error = ''
        return detail
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
        throw error
      } finally {
        this.loading = false
      }
    },

    async createGroup(payload: CreateGroupPayload) {
      this.loading = true
      try {
        const created = await createGroup(payload)
        this.groups.unshift(created)
        this.activeGroupId = created.id
        this.activeGroupDetail = created
        this.error = ''
        return created
      } catch (error) {
        if (error instanceof UnauthorizedError) throw error
        this.error = describeApiError(error)
        throw error
      } finally {
        this.loading = false
      }
    },

    async updateGroup(id: number, payload: UpdateGroupPayload) {
      this.loading = true
      try {
        const updated = await updateGroup(id, payload)
        const idx = this.groups.findIndex((g) => g.id === id)
        if (idx !== -1) {
          this.groups[idx] = { ...this.groups[idx], ...updated }
        }
        if (this.activeGroupDetail && this.activeGroupDetail.id === id) {
          this.activeGroupDetail = { ...this.activeGroupDetail, ...updated }
        }
        this.error = ''
        return updated
      } catch (error) {
        if (error instanceof UnauthorizedError) throw error
        this.error = describeApiError(error)
        throw error
      } finally {
        this.loading = false
      }
    },

    async deleteGroup(id: number) {
      this.loading = true
      try {
        await deleteGroup(id)
        this.groups = this.groups.filter((g) => g.id !== id)
        if (this.activeGroupId === id) {
          this.activeGroupId = null
          this.activeGroupDetail = null
          this.groupTasks = []
        }
        this.error = ''
      } catch (error) {
        if (error instanceof UnauthorizedError) throw error
        this.error = describeApiError(error)
        throw error
      } finally {
        this.loading = false
      }
    },

    async updateMembers(groupId: number, roleIds: number[]) {
      try {
        const members = await setGroupMembers(groupId, roleIds)
        if (this.activeGroupDetail && this.activeGroupDetail.id === groupId) {
          this.activeGroupDetail.members = members
          this.activeGroupDetail.memberCount = members.length
        }
        const idx = this.groups.findIndex((g) => g.id === groupId)
        if (idx !== -1) {
          this.groups[idx].memberCount = members.length
        }
        return members
      } catch (error) {
        if (error instanceof UnauthorizedError) throw error
        this.error = describeApiError(error)
        throw error
      }
    },

    async loadGroupTasks(groupId: number) {
      this.tasksLoading = true
      try {
        this.groupTasks = await listGroupTasks(groupId)
        this.taskError = ''
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.taskError = describeApiError(error)
      } finally {
        this.tasksLoading = false
      }
    },

    async createGroupTask(groupId: number, payload: CreateGroupTaskPayload) {
      try {
        const task = await createGroupTask(groupId, payload)
        this.groupTasks.unshift(task)
        if (this.activeGroupDetail && this.activeGroupDetail.id === groupId) {
          this.activeGroupDetail.taskCount += 1
        }
        const idx = this.groups.findIndex((g) => g.id === groupId)
        if (idx !== -1) {
          this.groups[idx].taskCount += 1
        }
        return task
      } catch (error) {
        if (error instanceof UnauthorizedError) throw error
        this.taskError = describeApiError(error)
        throw error
      }
    },

    async updateGroupTask(groupId: number, taskId: number, payload: UpdateGroupTaskPayload) {
      try {
        const updated = await updateGroupTask(groupId, taskId, payload)
        const idx = this.groupTasks.findIndex((t) => t.id === taskId)
        if (idx !== -1) {
          this.groupTasks[idx] = updated
        }
        return updated
      } catch (error) {
        if (error instanceof UnauthorizedError) throw error
        this.taskError = describeApiError(error)
        throw error
      }
    },

    async deleteGroupTask(groupId: number, taskId: number) {
      try {
        await deleteGroupTask(groupId, taskId)
        this.groupTasks = this.groupTasks.filter((t) => t.id !== taskId)
        if (this.activeGroupDetail && this.activeGroupDetail.id === groupId) {
          this.activeGroupDetail.taskCount = Math.max(0, this.activeGroupDetail.taskCount - 1)
        }
        const idx = this.groups.findIndex((g) => g.id === groupId)
        if (idx !== -1) {
          this.groups[idx].taskCount = Math.max(0, this.groups[idx].taskCount - 1)
        }
      } catch (error) {
        if (error instanceof UnauthorizedError) throw error
        this.taskError = describeApiError(error)
        throw error
      }
    },

    async loadTaskRuns(groupId: number, taskId: number) {
      try {
        this.activeTaskId = taskId
        this.activeTaskRuns = await listGroupTaskRuns(groupId, taskId)
        if (this.activeTaskRuns.length > 0) {
          this.activeRun = this.activeTaskRuns[0]
          await this.loadRunMessages(this.activeRun.id)
        } else {
          this.activeRun = null
          this.flowMessages = []
        }
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.taskError = describeApiError(error)
      }
    },

    async loadRunMessages(runId: number) {
      try {
        const msgs = await listGroupRunMessages(runId)
        this.flowMessages = msgs
        return msgs
      } catch (error) {
        if (error instanceof UnauthorizedError) return []
        console.error('Failed to load group run messages', error)
        return []
      }
    },

    resetFlowState() {
      if (this.unsubEvents) {
        this.unsubEvents()
        this.unsubEvents = null
      }
      this.flowMessages = []
      this.activeRun = null
    },

    async triggerTask(groupId: number, taskId: number, prompt?: string) {
      try {
        const run = await triggerGroupTask(groupId, taskId, prompt)
        this.activeRun = run
        this.activeTaskRuns.unshift(run)
        const taskIdx = this.groupTasks.findIndex((t) => t.id === taskId)
        if (taskIdx !== -1) {
          this.groupTasks[taskIdx].lastRunStatus = 'running'
        }
        this.subscribeRunEvents(run.id)
        return run
      } catch (error) {
        if (error instanceof UnauthorizedError) throw error
        this.taskError = describeApiError(error)
        throw error
      }
    },

    async sendUserMessage(runId: number, content: string) {
      try {
        const msg = await postGroupRunMessage(runId, content)
        this.flowMessages.push(msg)
        return msg
      } catch (error) {
        if (error instanceof UnauthorizedError) throw error
        throw error
      }
    },

    async cancelRun(runId: number) {
      try {
        await cancelGroupRun(runId)
        if (this.activeRun && this.activeRun.id === runId) {
          this.activeRun.status = 'cancelled'
        }
      } catch (error) {
        if (error instanceof UnauthorizedError) throw error
        throw error
      }
    },

    subscribeRunEvents(runId: number) {
      if (this.unsubEvents) {
        this.unsubEvents()
        this.unsubEvents = null
      }

      const runIdStr = String(runId)
      const handleFlowEvent = (envelope: any) => {
        if (envelope.channel !== 'tasks' || envelope.topic !== runIdStr) return

        const type = envelope.type
        const payload = envelope.payload as any

        if (type === 'message' && payload) {
          const msg = payload as FlowMessage
          const existingIdx = this.flowMessages.findIndex((m) => m.id === msg.id)
          if (existingIdx !== -1) {
            this.flowMessages[existingIdx] = msg
          } else {
            this.flowMessages.push(msg)
          }
        } else if (type === 'chunk' && payload) {
          const { messageId, text, reasoning } = payload
          const msg = this.flowMessages.find((m) => m.id === messageId)
          if (msg) {
            if (text) msg.content = (msg.content || '') + text
            if (reasoning) msg.reasoning = (msg.reasoning || '') + reasoning
            msg.status = 'streaming'
          }
        } else if (type === 'status' && payload) {
          const { status, currentStep } = payload
          if (this.activeRun && this.activeRun.id === runId) {
            this.activeRun.status = status
            if (currentStep !== undefined) this.activeRun.currentStep = currentStep
          }
        }
      }

      void unifiedEventBus.subscribe('tasks', runIdStr, handleFlowEvent)

      this.unsubEvents = () => {
        void unifiedEventBus.unsubscribe('tasks', runIdStr, handleFlowEvent)
      }
    },
  },
})

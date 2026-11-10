// Git worktrees per project. The list is shared by the project settings page,
// the project tree (for session labels) and the new-session form, so each
// project is fetched once and kept in step by create/remove actions.
import { defineStore } from 'pinia'
import {
  createProjectWorktree,
  listProjectWorktrees,
  removeProjectWorktree,
  type CreateWorktreeBody,
  type ProjectWorktree,
} from '../api/worktrees'

export const useWorktreesStore = defineStore('worktrees', {
  state: () => ({
    byProject: {} as Record<number, ProjectWorktree[]>,
    loadingByProject: {} as Record<number, boolean>,
    loadedByProject: {} as Record<number, boolean>,
    inFlight: {} as Record<number, Promise<void> | undefined>,
    errorByProject: {} as Record<number, string>,
    revision: 0,
  }),

  getters: {
    forProject: (state) => (projectId: number): ProjectWorktree[] =>
      state.byProject[projectId] ?? [],

    availableForProject: (state) => (projectId: number): ProjectWorktree[] =>
      (state.byProject[projectId] ?? []).filter((entry) => entry.available),

    byId: (state) => (projectId: number, worktreeId: number): ProjectWorktree | undefined =>
      (state.byProject[projectId] ?? []).find((entry) => entry.id === worktreeId),

    isLoading: (state) => (projectId: number): boolean =>
      state.loadingByProject[projectId] === true,
  },

  actions: {
    async load(projectId: number, force = false) {
      if (!projectId) return
      if (!force && this.loadedByProject[projectId]) return
      const pending = this.inFlight[projectId]
      if (pending) return pending

      const run = (async () => {
        this.loadingByProject = { ...this.loadingByProject, [projectId]: true }
        this.errorByProject = { ...this.errorByProject, [projectId]: '' }
        try {
          const list = await listProjectWorktrees(projectId)
          this.byProject = { ...this.byProject, [projectId]: list }
          this.loadedByProject = { ...this.loadedByProject, [projectId]: true }
        } catch (error) {
          this.errorByProject = {
            ...this.errorByProject,
            [projectId]: error instanceof Error ? error.message : '工作树加载失败',
          }
        } finally {
          this.loadingByProject = { ...this.loadingByProject, [projectId]: false }
          this.inFlight = { ...this.inFlight, [projectId]: undefined }
        }
      })()
      this.inFlight = { ...this.inFlight, [projectId]: run }
      return run
    },

    async create(projectId: number, body: CreateWorktreeBody): Promise<ProjectWorktree> {
      const created = await createProjectWorktree(projectId, body)
      this.byProject = {
        ...this.byProject,
        [projectId]: [...(this.byProject[projectId] ?? []), created],
      }
      this.loadedByProject = { ...this.loadedByProject, [projectId]: true }
      this.revision += 1
      return created
    },

    async remove(projectId: number, worktreeId: number): Promise<void> {
      await removeProjectWorktree(projectId, worktreeId)
      this.byProject = {
        ...this.byProject,
        [projectId]: (this.byProject[projectId] ?? []).filter((entry) => entry.id !== worktreeId),
      }
      this.revision += 1
    },

    invalidate(projectId: number) {
      this.loadedByProject = { ...this.loadedByProject, [projectId]: false }
      this.revision += 1
    },
  },
})

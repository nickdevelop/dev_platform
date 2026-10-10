// Project list for the navigation shell. An expired session is handled by the
// global 401 handler, so the store only records the error text for every other
// failure.
import { defineStore } from 'pinia'
import { describeApiError, UnauthorizedError } from '../api/client'
import { listProjects, type Project } from '../api/projects'

export const useProjectsStore = defineStore('projects', {
  state: () => ({
    projects: [] as Project[],
    loading: false,
    error: '',
  }),

  getters: {
    byId: (state) => (id: number): Project | undefined =>
      state.projects.find((project) => project.id === id),
  },

  actions: {
    async load() {
      this.loading = true
      try {
        const list = await listProjects()
        this.projects = list.projects
        this.error = ''
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
      } finally {
        this.loading = false
      }
    },
  },
})

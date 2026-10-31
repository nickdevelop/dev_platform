// Project list for the navigation shell. An expired session is handled by the
// global 401 handler, so the store only records the error text for every other
// failure.
import { defineStore } from 'pinia'
import { describeApiError, UnauthorizedError } from '../api/client'
import { listProjects, type Project } from '../api/projects'

const DISPLAY_CONFIG_KEY = 'dev-platform-projects-display-config'

export interface ProjectDisplayConfig {
  order: number[]
  hidden: number[]
}

function loadDisplayConfig(): ProjectDisplayConfig {
  try {
    const raw = localStorage.getItem(DISPLAY_CONFIG_KEY)
    if (raw) {
      const parsed = JSON.parse(raw) as Partial<ProjectDisplayConfig>
      return {
        order: Array.isArray(parsed.order) ? parsed.order : [],
        hidden: Array.isArray(parsed.hidden) ? parsed.hidden : [],
      }
    }
  } catch {
    // fallback on error
  }
  return { order: [], hidden: [] }
}

function saveDisplayConfig(config: ProjectDisplayConfig) {
  try {
    localStorage.setItem(DISPLAY_CONFIG_KEY, JSON.stringify(config))
  } catch {
    // best-effort persistence
  }
}

export const useProjectsStore = defineStore('projects', {
  state: () => ({
    projects: [] as Project[],
    loading: false,
    error: '',
    displayConfig: loadDisplayConfig() as ProjectDisplayConfig,
  }),

  getters: {
    byId: (state) => (id: number): Project | undefined =>
      state.projects.find((project) => project.id === id),

    orderedProjects: (state): Project[] => {
      const order = state.displayConfig.order
      const projectMap = new Map(state.projects.map((p) => [p.id, p]))
      const result: Project[] = []
      for (const id of order) {
        const p = projectMap.get(id)
        if (p) {
          result.push(p)
          projectMap.delete(id)
        }
      }
      for (const p of state.projects) {
        if (projectMap.has(p.id)) {
          result.push(p)
        }
      }
      return result
    },

    orderedVisibleProjects(): Project[] {
      const hiddenSet = new Set(this.displayConfig.hidden)
      return this.orderedProjects.filter((p) => !hiddenSet.has(p.id))
    },

    isHidden: (state) => (id: number): boolean => {
      return state.displayConfig.hidden.includes(id)
    },
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

    toggleProjectVisibility(id: number) {
      const hidden = new Set(this.displayConfig.hidden)
      if (hidden.has(id)) {
        hidden.delete(id)
      } else {
        hidden.add(id)
      }
      this.displayConfig.hidden = Array.from(hidden)
      saveDisplayConfig(this.displayConfig)
    },

    setProjectVisibility(id: number, visible: boolean) {
      const hidden = new Set(this.displayConfig.hidden)
      if (visible) {
        hidden.delete(id)
      } else {
        hidden.add(id)
      }
      this.displayConfig.hidden = Array.from(hidden)
      saveDisplayConfig(this.displayConfig)
    },

    moveProjectUp(id: number) {
      const current = [...this.orderedProjects]
      const index = current.findIndex((p) => p.id === id)
      if (index <= 0) return
      const temp = current[index - 1]
      current[index - 1] = current[index]
      current[index] = temp
      this.displayConfig.order = current.map((p) => p.id)
      saveDisplayConfig(this.displayConfig)
    },

    moveProjectDown(id: number) {
      const current = [...this.orderedProjects]
      const index = current.findIndex((p) => p.id === id)
      if (index === -1 || index >= current.length - 1) return
      const temp = current[index + 1]
      current[index + 1] = current[index]
      current[index] = temp
      this.displayConfig.order = current.map((p) => p.id)
      saveDisplayConfig(this.displayConfig)
    },

    reorderProjects(newOrder: number[]) {
      this.displayConfig.order = [...newOrder]
      saveDisplayConfig(this.displayConfig)
    },
  },
})

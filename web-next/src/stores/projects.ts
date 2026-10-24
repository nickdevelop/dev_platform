import { create } from 'zustand'
import { describeApiError, UnauthorizedError } from '../api/client'
import { listProjects, type Project } from '../api/projects'

interface ProjectsState {
  projects: Project[]
  loading: boolean
  error: string
  load: () => Promise<void>
  byId: (id: number) => Project | undefined
}

export const useProjectsStore = create<ProjectsState>((set, get) => ({
  projects: [],
  loading: false,
  error: '',

  load: async () => {
    set({ loading: true })
    try {
      const list = await listProjects()
      set({ projects: list.projects, error: '', loading: false })
    } catch (error) {
      if (error instanceof UnauthorizedError) {
        set({ loading: false })
        return
      }
      set({ error: describeApiError(error), loading: false })
    }
  },

  byId: (id) => get().projects.find((project) => project.id === id),
}))

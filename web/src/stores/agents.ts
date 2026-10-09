// Pinia store for a project's agent backends and instances. It owns the
// capability declarations (the source of truth for workbench controls) and the
// per-project instance list; stopping one instance never affects another.

import { defineStore } from 'pinia'
import {
  listAgentBackends,
  listProjectInstances,
  startProjectInstance,
  stopProjectInstance,
  type AgentBackend,
  type AgentInstance,
  type BackendId,
  type Capabilities,
} from '../api/agents'
import { describeApiError, UnauthorizedError } from '../api/client'
import { useAuthStore } from './auth'

export const useAgentsStore = defineStore('agents', {
  state: () => ({
    backends: [] as AgentBackend[],
    backendsLoaded: false,
    instances: [] as AgentInstance[],
    projectId: null as number | null,
    loading: false,
    error: '',
  }),

  getters: {
    // The declared capabilities for a backend, or null while unknown. A null
    // result means no control may be shown.
    capabilitiesFor:
      (state) =>
      (backend: BackendId | string): Capabilities | null =>
        state.backends.find((entry) => entry.backend === backend)?.capabilities ?? null,
    runningInstances: (state) => state.instances.filter((entry) => entry.status === 'running'),
  },

  actions: {
    // Loads the backend registry once per page load. The capability map is
    // static for a backend, so it does not need refetching per project.
    async loadBackends() {
      if (this.backendsLoaded) return
      try {
        const list = await listAgentBackends()
        this.backends = list.backends
        this.backendsLoaded = true
      } catch (error) {
        if (error instanceof UnauthorizedError) return // global handler went to login
        this.error = describeApiError(error)
      }
    },

    async loadInstances(projectId: number) {
      this.loading = true
      this.projectId = projectId
      try {
        const list = await listProjectInstances(projectId)
        this.instances = list.instances
        this.error = ''
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
      } finally {
        this.loading = false
      }
    },

    async startInstance(projectId: number, backend: BackendId) {
      const auth = useAuthStore()
      try {
        const created = await startProjectInstance(projectId, backend, auth.csrfToken)
        this.instances = [...this.instances, created]
        this.error = ''
        return created
      } catch (error) {
        if (error instanceof UnauthorizedError) return null
        this.error = describeApiError(error)
        // A failed start is still recorded by the backend; refresh so it is
        // visible without pretending the instance is running.
        await this.loadInstances(projectId)
        return null
      }
    },

    async stopInstance(projectId: number, instanceId: string) {
      const auth = useAuthStore()
      try {
        const stopped = await stopProjectInstance(projectId, instanceId, auth.csrfToken)
        this.instances = this.instances.map((entry) =>
          entry.id === instanceId ? stopped : entry,
        )
        this.error = ''
        return stopped
      } catch (error) {
        if (error instanceof UnauthorizedError) return null
        this.error = describeApiError(error)
        return null
      }
    },
  },
})

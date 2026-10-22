// Pinia store for a project's agent backends and instances. It owns the
// capability declarations (the source of truth for workbench controls) and the
// per-project instance list; stopping one instance never affects another.

import { defineStore } from 'pinia'
import {
  fetchAgentDiagnostics,
  getServiceStatus,
  listAgentBackends,
  listProjectInstances,
  startAgentService,
  startProjectInstance,
  stopAgentService,
  stopProjectInstance,
  type AgentBackend,
  type AgentInstance,
  type AgentServiceStatus,
  type BackendDiagnostics,
  type BackendId,
  type Capabilities,
} from '../api/agents'
import { describeApiError, UnauthorizedError } from '../api/client'
import { isHexColor } from '../shared/color'
import { useAuthStore } from './auth'

export const useAgentsStore = defineStore('agents', {
  state: () => ({
    backends: [] as AgentBackend[],
    backendsLoaded: false,
    instances: [] as AgentInstance[],
    projectId: null as number | null,
    // Per-backend startup diagnostics. Point-in-time results, refetched every
    // time the agents view loads; they are never cached like the capability
    // map, because executable/API state can change at any moment.
    diagnostics: [] as BackendDiagnostics[],
    diagnosticsLoading: false,
    diagnosticsError: '',
    loading: false,
    error: '',
    // Per-backend machine-level service status, as last observed by the
    // platform settings page. null means "not looked at yet" — deliberately not
    // "stopped": the platform never guesses about a third-party process.
    services: {} as Record<string, AgentServiceStatus | null>,
    serviceError: '',
  }),

  getters: {
    // The declared capabilities for a backend, or null while unknown. A null
    // result means no control may be shown.
    capabilitiesFor:
      (state) =>
      (backend: BackendId | string): Capabilities | null =>
        state.backends.find((entry) => entry.backend === backend)?.capabilities ?? null,
    // The backend entry for an id, or undefined while the list is unknown.
    backendFor:
      (state) =>
      (backend: BackendId | string): AgentBackend | undefined =>
        state.backends.find((entry) => entry.backend === backend),
    // Display label for a backend. An unknown backend shows its raw id, never
    // another backend's name.
    backendLabel() {
      return (backend: BackendId | string): string =>
        this.backendFor(backend)?.label || String(backend)
    },
    // Display visual tone preference for a backend tag.
    backendTone() {
      return (backend: BackendId | string): string =>
        this.backendFor(backend)?.tone || ''
    },
    // Exact brand colour declared by the backend descriptor, when it is a valid
    // #RRGGBB value. An invalid or absent colour falls back to the tone classes.
    backendColor() {
      return (backend: BackendId | string): string => {
        const color = this.backendFor(backend)?.color || ''
        return isHexColor(color) ? color : ''
      }
    },
    // Backends in the catalog's presentation order.
    orderedBackends: (state) =>
      [...state.backends].sort((a, b) => a.order - b.order),
    // Backends whose machine-level service the operator can start and stop.
    serviceBackends(): AgentBackend[] {
      return this.orderedBackends.filter((entry) => entry.serviceSupported)
    },
    runningInstances: (state) => state.instances.filter((entry) => entry.status === 'running'),
    // Whether a backend is enabled platform-wide. The platform setting is the
    // only enable/disable; an unknown backend defaults to enabled, matching the
    // server.
    backendEnabledFor:
      (state) =>
      (backend: BackendId | string): boolean =>
        state.backends.find((entry) => entry.backend === backend)?.enabled ?? true,
  },

  actions: {
    // Loads the backend registry once per page load. The capability map is
    // static for a backend, so it does not need refetching per project.
    async loadBackends() {
      if (this.backendsLoaded) return
      try {
        const list = await listAgentBackends()
        this.backends = list.backends ?? []
        this.backendsLoaded = true
      } catch (error) {
        if (error instanceof UnauthorizedError) return // global handler went to login
        this.error = describeApiError(error)
      }
    },

    // Loads the per-backend diagnostics report. A failure here is surfaced on
    // its own flag so it cannot mask the instance list's error state.
    async loadDiagnostics() {
      this.diagnosticsLoading = true
      try {
        const report = await fetchAgentDiagnostics()
        this.diagnostics = report.diagnostics
        this.diagnosticsError = ''
      } catch (error) {
        if (error instanceof UnauthorizedError) return // global handler went to login
        this.diagnosticsError = describeApiError(error)
      } finally {
        this.diagnosticsLoading = false
      }
    },

    // Detects one backend's machine-level service. Called on page load and by
    // the operator's "检测" button — never on a timer.
    async detectService(backend: BackendId): Promise<AgentServiceStatus | null> {
      try {
        const status = await getServiceStatus(backend)
        this.services = { ...this.services, [backend]: status }
        this.serviceError = ''
        return status
      } catch (error) {
        if (error instanceof UnauthorizedError) return null
        this.serviceError = describeApiError(error)
        return null
      }
    },

    // Starts the service because the operator asked. A service that is already
    // running is refused by the backend and reported as-is.
    async startService(backend: BackendId): Promise<AgentServiceStatus | null> {
      const auth = useAuthStore()
      try {
        const status = await startAgentService(backend, auth.csrfToken)
        this.services = { ...this.services, [backend]: status }
        this.serviceError = ''
        return status
      } catch (error) {
        if (error instanceof UnauthorizedError) return null
        this.serviceError = describeApiError(error)
        return null
      }
    },

    // Stops the service because the operator asked, after confirming what that
    // costs the other clients of the same service.
    async stopService(backend: BackendId): Promise<AgentServiceStatus | null> {
      const auth = useAuthStore()
      try {
        const status = await stopAgentService(backend, auth.csrfToken)
        this.services = { ...this.services, [backend]: status }
        this.serviceError = ''
        return status
      } catch (error) {
        if (error instanceof UnauthorizedError) return null
        this.serviceError = describeApiError(error)
        return null
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

    // Reloads the backend registry after a platform-level enable/disable change,
    // so the workbench sees the new state without a page reload.
    async refreshBackends() {
      this.backendsLoaded = false
      await this.loadBackends()
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

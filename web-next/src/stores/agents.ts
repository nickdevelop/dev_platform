import { create } from 'zustand'
import { describeApiError, UnauthorizedError } from '../api/client'
import {
  listAgentBackends,
  listProjectInstances,
  type AgentBackend,
  type AgentInstance,
  type BackendId,
  type Capabilities,
} from '../api/agents'
import { isHexColor } from '../shared/color'

interface AgentsState {
  backends: AgentBackend[]
  backendsLoaded: boolean
  instances: AgentInstance[]
  projectId: number | null
  loading: boolean
  error: string
  loadBackends: () => Promise<void>
  loadInstances: (projectId: number) => Promise<void>
  backendFor: (backend: BackendId | string) => AgentBackend | undefined
  backendLabel: (backend: BackendId | string) => string
  backendTone: (backend: BackendId | string) => string
  backendColor: (backend: BackendId | string) => string
  capabilitiesFor: (backend: BackendId | string) => Capabilities | null
  orderedBackends: () => AgentBackend[]
}

export const useAgentsStore = create<AgentsState>((set, get) => ({
  backends: [],
  backendsLoaded: false,
  instances: [],
  projectId: null,
  loading: false,
  error: '',

  loadBackends: async () => {
    if (get().backendsLoaded) return
    try {
      const list = await listAgentBackends()
      set({ backends: list.backends ?? [], backendsLoaded: true, error: '' })
    } catch (error) {
      if (error instanceof UnauthorizedError) return
      set({ error: describeApiError(error) })
    }
  },

  loadInstances: async (projectId) => {
    set({ loading: true })
    try {
      const list = await listProjectInstances(projectId)
      set({ instances: list.instances ?? [], projectId, error: '', loading: false })
    } catch (error) {
      if (error instanceof UnauthorizedError) {
        set({ loading: false })
        return
      }
      set({ error: describeApiError(error), loading: false })
    }
  },

  backendFor: (backend) => get().backends.find((entry) => entry.backend === backend),
  backendLabel: (backend) => get().backendFor(backend)?.label || String(backend),
  backendTone: (backend) => get().backendFor(backend)?.tone || '',
  backendColor: (backend) => {
    const color = get().backendFor(backend)?.color || ''
    return isHexColor(color) ? color : ''
  },
  capabilitiesFor: (backend) => get().backendFor(backend)?.capabilities ?? null,
  orderedBackends: () => [...get().backends].sort((a, b) => a.order - b.order),
}))

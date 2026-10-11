// Pinia store for model services and their models. Mutations reload the list so
// the UI always reflects effective usability (service and model enablement).
import { defineStore } from 'pinia'
import {
  addModel,
  createService,
  deleteModel,
  deleteService,
  fetchModels,
  listServices,
  testModel,
  updateModel,
  updateService,
  type ModelInput,
  type ModelTestResult,
  type ProbeInput,
  type RemoteModel,
  type ServiceDto,
  type ServiceInput,
  type TestInput,
} from '../api/models'
import { describeApiError, UnauthorizedError } from '../api/client'
import { useAuthStore } from './auth'

export const useModelsStore = defineStore('models', {
  state: () => ({
    services: [] as ServiceDto[],
    loading: false,
    error: '',
  }),

  getters: {
    serviceById: (state) => (id: number) => state.services.find((s) => s.id === id) ?? null,
  },

  actions: {
    async load() {
      this.loading = true
      try {
        const { services } = await listServices()
        this.services = services
        this.error = ''
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
      } finally {
        this.loading = false
      }
    },

    // saveService creates a service when id is omitted, otherwise updates it.
    async saveService(input: ServiceInput, id?: number): Promise<boolean> {
      try {
        if (id === undefined) await createService(input, useAuthStore().csrfToken)
        else await updateService(id, input, useAuthStore().csrfToken)
        await this.load()
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.error = describeApiError(error)
        return false
      }
    },

    async removeService(id: number): Promise<boolean> {
      try {
        await deleteService(id, useAuthStore().csrfToken)
        await this.load()
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.error = describeApiError(error)
        return false
      }
    },

    async addModelTo(serviceId: number, input: ModelInput): Promise<boolean> {
      try {
        await addModel(serviceId, input, useAuthStore().csrfToken)
        await this.load()
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.error = describeApiError(error)
        return false
      }
    },

    async saveModel(id: number, input: ModelInput): Promise<boolean> {
      try {
        await updateModel(id, input, useAuthStore().csrfToken)
        await this.load()
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.error = describeApiError(error)
        return false
      }
    },

    async removeModel(id: number): Promise<boolean> {
      try {
        await deleteModel(id, useAuthStore().csrfToken)
        await this.load()
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.error = describeApiError(error)
        return false
      }
    },

    // fetchRemote lists a service's models using the current (possibly
    // unsaved) form config. Returns null and sets error on failure.
    async fetchRemote(input: ProbeInput): Promise<RemoteModel[]> {
      try {
        const { models } = await fetchModels(input)
        this.error = ''
        return models
      } catch (error) {
        if (error instanceof UnauthorizedError) return []
        this.error = describeApiError(error)
        return []
      }
    },

    async test(input: TestInput): Promise<ModelTestResult | null> {
      try {
        const result = await testModel(input)
        this.error = ''
        return result
      } catch (error) {
        if (error instanceof UnauthorizedError) return null
        this.error = describeApiError(error)
        return null
      }
    },
  },
})

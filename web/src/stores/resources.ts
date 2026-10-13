// Pinia store for the canonical resources, their project associations and the
// managed-file sync state. It never stores a credential value: a write-only
// `secretValue` travels from the editor to the API and only the opaque
// secretRef is retained.
import { defineStore } from 'pinia'
import { describeApiError, UnauthorizedError } from '../api/client'
import {
  applyProjectSync,
  associateProjectResource,
  createResource,
  deleteResource,
  disassociateProjectResource,
  listProjectResources,
  listResources,
  previewProjectSync,
  resolveProjectSync,
  updateResource,
  type ResourceDefinition,
  type ResourceDefinitionBody,
  type SyncPreview,
} from '../api/resources'
import { useAuthStore } from './auth'

export const useResourcesStore = defineStore('resources', {
  state: () => ({
    resources: [] as ResourceDefinition[],
    projectResources: [] as ResourceDefinition[],
    previews: [] as SyncPreview[],
    loading: false,
    syncLoading: false,
    error: '',
  }),

  getters: {
    byId: (state) => (id: number): ResourceDefinition | undefined =>
      state.resources.find((resource) => resource.id === id),
    // Project-scoped prompts and skills/MCP associated with the project.
    isAssociated: (state) => (id: number): boolean =>
      state.projectResources.some((resource) => resource.id === id),
  },

  actions: {
    async loadAll() {
      this.loading = true
      try {
        const list = await listResources()
        this.resources = list.resources
        this.error = ''
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
      } finally {
        this.loading = false
      }
    },

    async loadProject(projectId: number) {
      this.loading = true
      try {
        const list = await listProjectResources(projectId)
        this.projectResources = list.resources
        this.error = ''
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
      } finally {
        this.loading = false
      }
    },

    async create(body: ResourceDefinitionBody): Promise<boolean> {
      try {
        await createResource(body, useAuthStore().csrfToken)
        await this.loadAll()
        this.error = ''
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.error = describeApiError(error)
        return false
      }
    },

    async update(id: number, body: ResourceDefinitionBody): Promise<boolean> {
      try {
        await updateResource(id, body, useAuthStore().csrfToken)
        await this.loadAll()
        this.error = ''
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.error = describeApiError(error)
        return false
      }
    },

    async remove(id: number): Promise<boolean> {
      try {
        await deleteResource(id, useAuthStore().csrfToken)
        this.resources = this.resources.filter((resource) => resource.id !== id)
        this.projectResources = this.projectResources.filter((resource) => resource.id !== id)
        this.error = ''
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.error = describeApiError(error)
        return false
      }
    },

    async associate(projectId: number, resourceId: number): Promise<boolean> {
      return this.changeAssociation(projectId, resourceId, true)
    },

    async disassociate(projectId: number, resourceId: number): Promise<boolean> {
      return this.changeAssociation(projectId, resourceId, false)
    },

    async changeAssociation(
      projectId: number,
      resourceId: number,
      associated: boolean,
    ): Promise<boolean> {
      try {
        const csrfToken = useAuthStore().csrfToken
        if (associated) {
          await associateProjectResource(projectId, resourceId, csrfToken)
        } else {
          await disassociateProjectResource(projectId, resourceId, csrfToken)
        }
        await this.loadProject(projectId)
        this.error = ''
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.error = describeApiError(error)
        return false
      }
    },

    async preview(projectId: number): Promise<boolean> {
      this.syncLoading = true
      try {
        const result = await previewProjectSync(projectId)
        this.previews = result.previews
        this.error = ''
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.error = describeApiError(error)
        return false
      } finally {
        this.syncLoading = false
      }
    },

    async apply(projectId: number, force: boolean): Promise<boolean> {
      this.syncLoading = true
      try {
        const result = await applyProjectSync(projectId, force, useAuthStore().csrfToken)
        this.previews = result.previews
        this.error = ''
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.error = describeApiError(error)
        return false
      } finally {
        this.syncLoading = false
      }
    },

    async resolve(projectId: number, resourceId: number): Promise<boolean> {
      this.syncLoading = true
      try {
        const result = await resolveProjectSync(projectId, resourceId, useAuthStore().csrfToken)
        const resolved = result.previews
        // Merge the resolved resource's results back into the list.
        this.previews = this.previews
          .filter((preview) => preview.resourceId !== resourceId)
          .concat(resolved)
        this.error = ''
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.error = describeApiError(error)
        return false
      } finally {
        this.syncLoading = false
      }
    },
  },
})

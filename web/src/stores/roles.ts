// Store for Agent Role Management.
import { defineStore } from 'pinia'
import { describeApiError, UnauthorizedError } from '../api/client'
import {
  listRoles,
  getRole,
  createRole,
  updateRole,
  deleteRole,
  type AgentRole,
  type CreateRolePayload,
  type UpdateRolePayload,
} from '../api/roles'

export const useRolesStore = defineStore('roles', {
  state: () => ({
    roles: [] as AgentRole[],
    currentRole: null as AgentRole | null,
    loading: false,
    error: '',
  }),

  getters: {
    byId: (state) => (id: number): AgentRole | undefined =>
      state.roles.find((r) => r.id === id),
  },

  actions: {
    async loadRoles(keyword?: string) {
      this.loading = true
      try {
        this.roles = await listRoles(keyword)
        this.error = ''
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
      } finally {
        this.loading = false
      }
    },

    async loadRole(id: number) {
      this.loading = true
      try {
        const role = await getRole(id)
        this.currentRole = role
        const idx = this.roles.findIndex((r) => r.id === id)
        if (idx !== -1) {
          this.roles[idx] = role
        } else {
          this.roles.unshift(role)
        }
        this.error = ''
        return role
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
        throw error
      } finally {
        this.loading = false
      }
    },

    async createRole(payload: CreateRolePayload) {
      this.loading = true
      try {
        const role = await createRole(payload)
        this.roles.push(role)
        this.error = ''
        return role
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
        throw error
      } finally {
        this.loading = false
      }
    },

    async updateRole(id: number, payload: UpdateRolePayload) {
      this.loading = true
      try {
        const updated = await updateRole(id, payload)
        const idx = this.roles.findIndex((r) => r.id === id)
        if (idx !== -1) {
          this.roles[idx] = updated
        }
        if (this.currentRole?.id === id) {
          this.currentRole = updated
        }
        this.error = ''
        return updated
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
        throw error
      } finally {
        this.loading = false
      }
    },

    async deleteRole(id: number) {
      this.loading = true
      try {
        await deleteRole(id)
        this.roles = this.roles.filter((r) => r.id !== id)
        if (this.currentRole?.id === id) {
          this.currentRole = null
        }
        this.error = ''
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
        throw error
      } finally {
        this.loading = false
      }
    },
  },
})

import { defineStore } from 'pinia'

export interface ProjectTerminalInstance {
  id: string
  cwd: string
  projectName: string
  tabId: string
  isMinimized: boolean
  left: number
  top: number
  zIndex: number
}

let counter = 1
const BASE_Z_INDEX = 60

export const useProjectTerminalStore = defineStore('projectTerminal', {
  state: () => ({
    instances: [] as ProjectTerminalInstance[],
    maxZIndex: BASE_Z_INDEX,
  }),

  getters: {
    hasActiveTerminal: (state) => state.instances.length > 0,
    isOpen: (state) => state.instances.length > 0,
    isMinimized: (state) => state.instances.length > 0 && state.instances.every((i) => i.isMinimized),
    minimizedInstances: (state) => state.instances.filter((i) => i.isMinimized),
    // Backward compatibility for single-instance tests
    cwd: (state) => state.instances[0]?.cwd ?? '',
    projectName: (state) => state.instances[0]?.projectName ?? '',
    tabId: (state) => state.instances[0]?.tabId ?? '',
  },

  actions: {
    openTerminal(cwd: string, projectName?: string): string {
      const name = projectName || '项目'

      // If an instance with the exact same cwd already exists, restore and bring it to front
      const existing = this.instances.find((i) => i.cwd === cwd)
      if (existing) {
        existing.isMinimized = false
        existing.projectName = name
        this.bringToFront(existing.id)
        return existing.id
      }

      // Calculate initial position with cascading offset
      const cascadeIndex = this.instances.length % 6
      const cascadeOffset = cascadeIndex * 24

      let baseLeft = 60
      let baseTop = 60
      if (typeof window !== 'undefined') {
        baseLeft = Math.max(16, Math.round((window.innerWidth - 800) / 2))
        baseTop = Math.max(16, Math.round((window.innerHeight - 500) / 2))
      }

      const left = baseLeft + cascadeOffset
      const top = baseTop + cascadeOffset
      this.maxZIndex++

      const id = `pt-${Date.now()}-${counter++}`
      const newInstance: ProjectTerminalInstance = {
        id,
        cwd,
        projectName: name,
        tabId: `modal-term-${Date.now()}-${counter++}`,
        isMinimized: false,
        left,
        top,
        zIndex: this.maxZIndex,
      }

      this.instances.push(newInstance)
      return id
    },

    bringToFront(id: string) {
      const inst = this.instances.find((i) => i.id === id)
      if (inst) {
        this.maxZIndex++
        inst.zIndex = this.maxZIndex
      }
    },

    minimize(id?: string) {
      if (id) {
        const inst = this.instances.find((i) => i.id === id)
        if (inst) inst.isMinimized = true
      } else if (this.instances.length > 0) {
        this.instances[this.instances.length - 1].isMinimized = true
      }
    },

    restore(id?: string) {
      if (id) {
        const inst = this.instances.find((i) => i.id === id)
        if (inst) {
          inst.isMinimized = false
          this.bringToFront(id)
        }
      } else {
        const minimized = this.instances.find((i) => i.isMinimized)
        if (minimized) {
          minimized.isMinimized = false
          this.bringToFront(minimized.id)
        }
      }
    },

    updatePosition(id: string, left: number, top: number) {
      const inst = this.instances.find((i) => i.id === id)
      if (inst) {
        inst.left = left
        inst.top = top
      }
    },

    closeTerminal(id?: string) {
      if (id) {
        this.instances = this.instances.filter((i) => i.id !== id)
      } else if (this.instances.length > 0) {
        this.instances.pop()
      }
    },
  },
})

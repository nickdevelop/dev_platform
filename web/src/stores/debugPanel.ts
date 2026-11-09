// The debug panel store. The settings toggle (enabled) only controls whether the
// floating launch button is visible; capture runs exclusively while the drawer
// is open — closing it detaches the envelope tap and wipes the buffer, so no
// history survives anywhere (localStorage only remembers the toggle).

import { defineStore } from 'pinia'
import type { EventEnvelope } from '../api/events'
import { unifiedEventBus, type StreamStatus } from '../shared/events/UnifiedEventBusClient'
import {
  appendEntry,
  createEntry,
  emptyDebugFilter,
  extractIdentities,
  filterEntries,
  type DebugEntry,
  type DebugEntryIdentities,
  type DebugFilter,
} from '../features/debug/debugModel'

// Per-browser local preference only: the debug panel is a developer aid, not a
// platform-wide product capability, so it intentionally does not touch the
// settings backend.
const ENABLED_STORAGE_KEY = 'dev-platform-debug-panel-v1'

function readEnabledFromStorage(): boolean {
  try {
    return localStorage.getItem(ENABLED_STORAGE_KEY) === '1'
  } catch {
    return false
  }
}

interface DebugRow extends DebugEntry {
  identity: DebugEntryIdentities
  // Expanded rows render prettyJSON once, lazily.
  pretty: string
}

export const useDebugPanelStore = defineStore('debugPanel', {
  state: () => ({
    enabled: readEnabledFromStorage(),
    open: false,
    entries: [] as DebugRow[],
    // Frames evicted by the ring buffer since the drawer opened.
    dropped: 0,
    filter: emptyDebugFilter(),
    status: 'idle' as StreamStatus,
    seq: 0,
    // Detach handles for the tap and the status listener (non-reactive usage only).
    offTap: null as (() => void) | null,
    offStatus: null as (() => void) | null,
  }),

  getters: {
    visibleEntries(state): DebugRow[] {
      return filterEntries(state.entries, state.filter) as DebugRow[]
    },
  },

  actions: {
    setEnabled(on: boolean) {
      this.enabled = on
      try {
        localStorage.setItem(ENABLED_STORAGE_KEY, on ? '1' : '0')
      } catch {
        // A locked-down localStorage degrades the toggle to session-only.
      }
      if (!on) this.closeDrawer()
    },

    // openDrawer attaches the envelope tap: from this exact point on, every
    // frame dispatched in this tab is captured.
    openDrawer() {
      if (!this.enabled || this.open) return
      this.entries = []
      this.dropped = 0
      this.filter = emptyDebugFilter()
      this.seq = 0
      this.status = unifiedEventBus.getStatus()
      this.offTap = unifiedEventBus.addEnvelopeTap((envelope) => this.recordEnvelope(envelope))
      this.offStatus = unifiedEventBus.onStatusChange((status) => {
        this.status = status
      })
      this.open = true
    },

    // closeDrawer is the hard stop: detach, then clear everything so no
    // capture data survives a closed panel.
    closeDrawer() {
      this.open = false
      if (this.offTap) {
        this.offTap()
        this.offTap = null
      }
      if (this.offStatus) {
        this.offStatus()
        this.offStatus = null
      }
      this.entries = []
      this.dropped = 0
      this.filter = emptyDebugFilter()
      this.seq = 0
    },

    toggleDrawer() {
      if (this.open) this.closeDrawer()
      else this.openDrawer()
    },

    clearBuffer() {
      this.entries = []
      this.dropped = 0
    },

    // recordEnvelope materializes one frame. Keep it linear and allocation-light:
    // references in, one pretty-print deferred to expand time. Hard guard on
    // drawer state: capture only runs while the panel is open, even if some
    // stale tap ever survives.
    recordEnvelope(envelope: EventEnvelope) {
      if (!this.open || !envelope || !envelope.channel) return
      this.seq += 1
      const base = createEntry(this.seq, envelope)
      const entry: DebugRow = {
        ...base,
        identity: extractIdentities(base),
        pretty: '',
      }
      this.dropped += appendEntry(this.entries, entry)
    },
  },
})

export type { DebugRow, DebugFilter }

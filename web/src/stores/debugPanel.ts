// The debug panel store. The settings toggle (enabled) only controls whether the
// floating launch button is visible. Capture itself is explicit: the panel's
// start/stop controls attach or detach the envelope tap, and the drawer's
// visibility is independent — closing the panel neither stops capture nor
// clears the captured frames. Only "清空" (or a page reload) drops the buffer.

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
    // Drawer visibility only; it never controls capture.
    open: false,
    // Capture switch: the tap is attached exactly while this is true.
    capturing: false,
    entries: [] as DebugRow[],
    // Frames evicted by the ring buffer since the last clear.
    dropped: 0,
    filter: emptyDebugFilter(),
    status: 'idle' as StreamStatus,
    // Local capture index for list keys / expansion ids, not the wire sequence.
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
      if (!on) {
        // Hiding the entry must not leave a silent background capture behind,
        // but the frames already captured stay available until 清空 / reload.
        this.stopCapture()
        this.closeDrawer()
      }
    },

    // openDrawer only shows the panel. It never starts capture and never clears
    // the buffer, so reopening shows exactly what was captured before.
    openDrawer() {
      if (!this.enabled || this.open) return
      this.status = unifiedEventBus.getStatus()
      this.open = true
    },

    // closeDrawer only hides the panel. A running capture keeps its tap and
    // buffer; a stopped capture simply stays stopped.
    closeDrawer() {
      this.open = false
    },

    // startCapture attaches the envelope tap. Existing frames are kept and new
    // frames append, so stop/start works as a pause/resume rather than a reset.
    startCapture() {
      if (this.capturing) return
      // Defensive: clear any stale listener before attaching a new pair.
      this.offTap?.()
      this.offStatus?.()
      this.offTap = null
      this.offStatus = null

      this.status = unifiedEventBus.getStatus()
      this.capturing = true
      this.offTap = unifiedEventBus.addEnvelopeTap((envelope) => this.recordEnvelope(envelope))
      this.offStatus = unifiedEventBus.onStatusChange((status) => {
        this.status = status
      })
    },

    // stopCapture detaches the tap and the status listener but keeps every
    // captured frame, the dropped counter and the active filter.
    stopCapture() {
      this.capturing = false
      this.status = unifiedEventBus.getStatus()
      this.offTap?.()
      this.offStatus?.()
      this.offTap = null
      this.offStatus = null
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
    // references in, one pretty-print deferred to expand time. The hard guard is
    // the capture switch, so a stale tap can never record while stopped.
    recordEnvelope(envelope: EventEnvelope) {
      if (!this.capturing || !envelope || !envelope.channel) return
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

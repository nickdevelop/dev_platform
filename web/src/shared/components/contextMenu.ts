// Types for the shared context menu (`ContextMenu.vue`).
//
// Element Plus has no right-click menu: `el-dropdown` with `trigger="contextmenu"`
// would need one wrapper per tree row (a workspace tree renders hundreds of rows),
// and a single virtual-triggered instance only positions at the reference element's
// box, not at the cursor. So the platform owns one small menu component and the
// spec's component table registers it as the context-menu answer (see
// docs/architecture/frontend-ui-system.md §五).
//
// A caller describes the menu as a list of items and remembers where the user
// right-clicked; the component owns placement and dismissal.

export type ContextMenuIcon = 'open' | 'folder' | 'upload' | 'download' | 'copy' | 'trash'

export interface ContextMenuItem {
  label: string
  // One of the menu's own vector icons; omit for a text-only row.
  icon?: ContextMenuIcon
  // Destructive actions: rendered in the danger colour, and a hairline is drawn
  // above them so they never sit flush against the ordinary items.
  danger?: boolean
  divider?: boolean
  disabled?: boolean
  onSelect: () => void
}

// Where the menu opens: the click's viewport coordinates. The component clamps
// the box back inside the viewport, so callers pass the raw cursor position.
export interface ContextMenuAnchor {
  x: number
  y: number
}

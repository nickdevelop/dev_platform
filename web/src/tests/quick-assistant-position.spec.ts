// Pure geometry for the draggable quick-assistant launcher: the button keeps
// 50px of vertical breathing room, and the 480x600 panel flips above the button
// when there is no room below.
import { describe, expect, it } from 'vitest'
import {
  clampButtonPosition,
  placePanel,
} from '../features/chat/quick/useDraggablePosition'

const viewport = { width: 1280, height: 800 }
const button = { width: 110, height: 36 }

describe('quick assistant position', () => {
  it('allows dragging across the whole viewport with a small edge margin', () => {
    expect(clampButtonPosition({ left: 100, top: 0 }, button, viewport).top).toBe(12)
    expect(clampButtonPosition({ left: 100, top: 9999 }, button, viewport).top).toBe(
      viewport.height - 12 - button.height,
    )
    expect(clampButtonPosition({ left: -200, top: 100 }, button, viewport).left).toBe(12)
    expect(clampButtonPosition({ left: 9999, top: 100 }, button, viewport).left).toBe(
      viewport.width - 12 - button.width,
    )
  })

  it('places the panel below the button with a 480x600 size', () => {
    const placed = placePanel({ left: 1000, top: 50, ...button }, viewport)
    expect(placed).toEqual({ left: 630, top: 98, width: 480, height: 600 })
  })

  it('flips the panel above the button near the bottom edge', () => {
    const placed = placePanel({ left: 1000, top: 700, ...button }, viewport)
    expect(placed.top).toBe(700 - 12 - 600)
    expect(placed.height).toBe(600)
  })

  it('shrinks the panel when the viewport is smaller than 480x600', () => {
    const placed = placePanel(
      { left: 200, top: 100, ...button },
      { width: 400, height: 500 },
    )
    expect(placed.width).toBe(400 - 24)
    expect(placed.height).toBe(500 - 24)
    expect(placed.left).toBe(12)
    expect(placed.top).toBe(12)
  })
})

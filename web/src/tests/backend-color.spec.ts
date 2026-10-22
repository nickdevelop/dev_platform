// Unit tests for the catalog brand-colour guard. The store filters the value
// before it reaches an inline CSS custom property, so this is the one place
// that decides which descriptor values are safe to render.
import { describe, expect, it } from 'vitest'
import { isHexColor } from '../shared/color'

describe('brand colour guard', () => {
  it('accepts only #RRGGBB values', () => {
    expect(isHexColor('#10B981')).toBe(true)
    expect(isHexColor('#abcdef')).toBe(true)
    expect(isHexColor('red')).toBe(false)
    expect(isHexColor('#fff')).toBe(false)
    expect(isHexColor('#10B981; position: fixed')).toBe(false)
  })
})

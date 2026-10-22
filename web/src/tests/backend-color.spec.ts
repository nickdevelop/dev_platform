// Unit tests for the catalog brand-colour helpers. The tag renders a solid
// fill, so the foreground must be chosen from luminance rather than per backend.
import { describe, expect, it } from 'vitest'
import { contrastTextColor, isHexColor } from '../shared/color'

describe('brand colour helpers', () => {
  it('accepts only #RRGGBB values', () => {
    expect(isHexColor('#10B981')).toBe(true)
    expect(isHexColor('#abcdef')).toBe(true)
    expect(isHexColor('red')).toBe(false)
    expect(isHexColor('#fff')).toBe(false)
    expect(isHexColor('#10B981; position: fixed')).toBe(false)
  })

  it('picks dark ink for the light emerald and orange brands', () => {
    expect(contrastTextColor('#10B981')).toBe('#0b0d11')
    expect(contrastTextColor('#F97316')).toBe('#0b0d11')
  })

  it('picks white ink for the dark navy brand', () => {
    expect(contrastTextColor('#1E40AF')).toBe('#ffffff')
  })

  it('falls back to white for an invalid value instead of throwing', () => {
    expect(contrastTextColor('red')).toBe('#ffffff')
  })
})

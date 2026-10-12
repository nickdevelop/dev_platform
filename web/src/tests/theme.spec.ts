// Unit tests for the appearance application layer: it maps an Appearance config
// onto the document's CSS custom properties. The font face is not part of that
// config — the built-in fonts stylesheet owns it — so these tests also pin that
// applying an appearance never reaches for a remote stylesheet.
import { afterEach, describe, expect, it } from 'vitest'
import { applyAppearance, CACHE_KEY } from '../theme/apply'
import { densities, themes } from '../theme/presets'

afterEach(() => {
  const root = document.documentElement
  root.removeAttribute('data-theme')
  root.removeAttribute('data-density')
  root.style.removeProperty('--font-family')
  root.style.removeProperty('--font-size-base')
  document.head.querySelectorAll('link').forEach((link) => link.remove())
  localStorage.clear()
})

describe('theme presets', () => {
  it('offers four themes and three densities', () => {
    expect(themes.map((t) => t.key)).toEqual(['light', 'dark', 'warm', 'cool'])
    expect(densities.map((d) => d.key)).toEqual(['compact', 'standard', 'relaxed'])
  })
})

describe('applyAppearance', () => {
  it('sets theme, density and font size on the root element', () => {
    applyAppearance({ theme: 'dark', fontSize: 16, density: 'compact' })

    const root = document.documentElement
    expect(root.dataset.theme).toBe('dark')
    expect(root.dataset.density).toBe('compact')
    expect(root.style.getPropertyValue('--font-size-base')).toBe('16px')
  })

  it('leaves the built-in font family alone', () => {
    applyAppearance({ theme: 'light', fontSize: 14, density: 'standard' })

    expect(document.documentElement.style.getPropertyValue('--font-family')).toBe('')
  })

  it('adds no stylesheet link', () => {
    applyAppearance({ theme: 'light', fontSize: 14, density: 'standard' })

    expect(document.head.querySelectorAll('link')).toHaveLength(0)
  })

  it('exposes a stable cache key', () => {
    expect(CACHE_KEY).toBe('platform.appearance')
  })
})

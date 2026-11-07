// Unit tests for the appearance application layer: it maps an Appearance config
// onto the document's CSS custom properties. The font face is not part of that
// config — the built-in fonts stylesheet owns it — so these tests also pin that
// applying an appearance never reaches for a remote stylesheet.
import { afterEach, describe, expect, it } from 'vitest'
import { applyAppearance, CACHE_KEY } from '../theme/apply'
import { fontSizes, resolveFontSizeKey, themes } from '../theme/presets'

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
  it('offers three light-based themes', () => {
    expect(themes.map((t) => t.key)).toEqual(['light', 'warm', 'cool'])
  })

  it('offers three font size tiers of 12 / 14 / 16 px', () => {
    expect(fontSizes.map((f) => f.label)).toEqual(['小', '标准', '大'])
    expect(fontSizes.map((f) => f.value)).toEqual([12, 14, 16])
  })

  it('resolves preset and in-between sizes to the nearest tier', () => {
    expect(resolveFontSizeKey(12)).toBe('small')
    expect(resolveFontSizeKey(13)).toBe('small')
    expect(resolveFontSizeKey(14)).toBe('medium')
    expect(resolveFontSizeKey(15)).toBe('medium')
    expect(resolveFontSizeKey(16)).toBe('large')
  })
})

describe('applyAppearance', () => {
  it('sets theme and font size on the root element', () => {
    applyAppearance({ theme: 'warm', fontSize: 16 })

    const root = document.documentElement
    expect(root.dataset.theme).toBe('warm')
    expect(root.dataset.density).toBeUndefined()
    expect(root.style.getPropertyValue('--font-size-base')).toBe('16px')
  })

  it('leaves the fixed layout tokens to the stylesheet', () => {
    applyAppearance({ theme: 'light', fontSize: 14 })

    const root = document.documentElement
    expect(root.style.getPropertyValue('--space-unit')).toBe('')
    expect(root.style.getPropertyValue('--control-height')).toBe('')
    expect(root.style.getPropertyValue('--radius')).toBe('')
  })

  it('falls back to light when an unknown or removed theme is provided', () => {
    applyAppearance({ theme: 'dark', fontSize: 14 })

    const root = document.documentElement
    expect(root.dataset.theme).toBe('light')
  })

  it('leaves the built-in font family alone', () => {
    applyAppearance({ theme: 'light', fontSize: 14 })

    expect(document.documentElement.style.getPropertyValue('--font-family')).toBe('')
  })

  it('adds no stylesheet link', () => {
    applyAppearance({ theme: 'light', fontSize: 14 })

    expect(document.head.querySelectorAll('link')).toHaveLength(0)
  })

  it('exposes a stable cache key', () => {
    expect(CACHE_KEY).toBe('platform.appearance')
  })
})

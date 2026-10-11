// Unit tests for the appearance application layer: it maps an Appearance
// config onto the document's CSS custom properties and manages the optional
// Google Fonts stylesheet link.
import { afterEach, describe, expect, it } from 'vitest'
import { applyAppearance, CACHE_KEY } from '../theme/apply'
import { densities, fonts, googleFontURL, themes } from '../theme/presets'

afterEach(() => {
  const root = document.documentElement
  root.removeAttribute('data-theme')
  root.removeAttribute('data-density')
  root.style.removeProperty('--font-family')
  root.style.removeProperty('--font-size-base')
  document.getElementById('platform-google-font')?.remove()
  localStorage.clear()
})

describe('theme presets', () => {
  it('offers four themes, four fonts and three densities', () => {
    expect(themes.map((t) => t.key)).toEqual(['light', 'dark', 'warm', 'cool'])
    expect(fonts.map((f) => f.key)).toEqual(['system', 'sans', 'serif', 'mono'])
    expect(densities.map((d) => d.key)).toEqual(['compact', 'standard', 'relaxed'])
  })

  it('builds a Google Fonts URL with + for spaces', () => {
    expect(googleFontURL('Noto Sans SC')).toBe(
      'https://fonts.googleapis.com/css2?family=Noto+Sans+SC&display=swap',
    )
  })
})

describe('applyAppearance', () => {
  it('sets theme, density, font family and font size on the root element', () => {
    applyAppearance({ theme: 'dark', font: 'sans', customFont: '', fontSize: 16, density: 'compact' })

    const root = document.documentElement
    expect(root.dataset.theme).toBe('dark')
    expect(root.dataset.density).toBe('compact')
    expect(root.style.getPropertyValue('--font-size-base')).toBe('16px')
    expect(root.style.getPropertyValue('--font-family')).toContain('Noto Sans SC')
  })

  it('injects a Google Fonts link for a custom font and removes it for a preset', () => {
    applyAppearance({ theme: 'light', font: 'custom', customFont: 'Noto Sans SC', fontSize: 14, density: 'standard' })

    const link = document.getElementById('platform-google-font') as HTMLLinkElement | null
    expect(link).not.toBeNull()
    expect(link?.href).toContain('family=Noto+Sans+SC')
    expect(document.documentElement.style.getPropertyValue('--font-family')).toContain('Noto Sans SC')

    applyAppearance({ theme: 'light', font: 'system', customFont: 'Noto Sans SC', fontSize: 14, density: 'standard' })
    expect(document.getElementById('platform-google-font')).toBeNull()
  })

  it('does not inject a link for a custom font with an empty family', () => {
    applyAppearance({ theme: 'light', font: 'custom', customFont: '', fontSize: 14, density: 'standard' })
    expect(document.getElementById('platform-google-font')).toBeNull()
  })

  it('exposes a stable cache key', () => {
    expect(CACHE_KEY).toBe('platform.appearance')
  })
})

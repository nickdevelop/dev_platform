import { describe, expect, it } from 'vitest'
import { theme as antdTheme } from 'antd'
import { applyAppearance } from '../theme/apply'
import { buildAntdTheme } from '../theme/antd'
import { densities, themes, type Appearance } from '../theme/presets'

const base: Appearance = { theme: 'light', fontSize: 14, density: 'standard' }

describe('theme bridge', () => {
  it('projects every preset onto CSS variables and antd tokens', () => {
    for (const preset of themes) {
      const appearance = { ...base, theme: preset.key }
      applyAppearance(appearance)
      const config = buildAntdTheme(appearance)

      expect(document.documentElement.dataset.theme).toBe(preset.key)
      expect(document.documentElement.style.getPropertyValue('--color-primary')).toBe(
        preset.colors.primary,
      )
      expect(config.token?.colorPrimary).toBe(preset.colors.primary)
      expect(config.token?.colorBgContainer).toBe(preset.colors.surface)
      expect(config.token?.colorText).toBe(preset.colors.text)
      expect(config.algorithm).toBe(preset.dark ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm)
    }
  })

  it('maps all three densities to control height and radius', () => {
    for (const density of densities) {
      const config = buildAntdTheme({ ...base, density: density.key })
      expect(config.token?.controlHeight).toBe(Number.parseFloat(density.controlHeight))
      expect(config.token?.borderRadius).toBe(Number.parseFloat(density.radius))
    }
  })

  it('keeps the font size configurable within the existing range', () => {
    const config = buildAntdTheme({ ...base, fontSize: 17 })
    expect(config.token?.fontSize).toBe(17)
  })
})

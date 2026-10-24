import { theme as antdTheme, type ThemeConfig } from 'antd'
import { densities, themes, type Appearance, type ThemeColors } from './presets'

// P0 theme bridge: one preset is projected onto both the document's CSS custom
// properties (apply.ts) and antd's alias tokens. Keeping the mapping here makes
// the dual-track risk visible: if a token is missing from this file, antd falls
// back to a derived colour that may not match the platform palette.
function px(value: string): number {
  return Number.parseFloat(value)
}

function tokensFor(colors: ThemeColors): ThemeConfig['token'] {
  return {
    colorPrimary: colors.primary,
    colorPrimaryHover: colors.primaryHover,
    colorBgBase: colors.bg,
    colorBgLayout: colors.bg,
    colorBgContainer: colors.surface,
    colorBgElevated: colors.surface,
    colorText: colors.text,
    colorTextSecondary: colors.textMuted,
    colorTextTertiary: colors.textMuted,
    colorBorder: colors.border,
    colorBorderSecondary: colors.border,
    colorError: colors.danger,
    colorErrorBorder: colors.dangerBorder,
    colorSuccess: colors.success,
    colorSuccessBorder: colors.successBorder,
    colorWarning: colors.warning,
    colorWarningBorder: colors.warningBorder,
    colorInfo: colors.info,
    colorInfoBorder: colors.infoBorder,
    colorLink: colors.primary,
    colorLinkHover: colors.primaryHover,
    colorBgMask: 'rgba(15, 23, 42, 0.45)',
  }
}

export function buildAntdTheme(appearance: Appearance): ThemeConfig {
  const preset = themes.find((entry) => entry.key === appearance.theme) ?? themes[0]
  const density = densities.find((entry) => entry.key === appearance.density) ?? densities[1]
  const colors = preset.colors
  const radius = px(density.radius)
  const controlHeight = px(density.controlHeight)
  const fontFamily = "'Google Sans Code', 'Noto Sans SC', sans-serif"

  return {
    // In v6 CSS variables are the default; naming the key makes the generated
    // variables predictable for the P0 inspection page.
    cssVar: { key: 'platform' },
    algorithm: preset.dark ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
    token: {
      ...tokensFor(colors),
      borderRadius: radius,
      borderRadiusLG: radius + 2,
      borderRadiusSM: Math.max(2, radius - 1),
      controlHeight,
      fontFamily,
      fontFamilyCode: "'Google Sans Code', 'Noto Sans SC', ui-monospace, monospace",
      fontSize: appearance.fontSize,
    },
    components: {
      Button: {
        primaryShadow: 'none',
        defaultShadow: 'none',
      },
      Table: {
        headerBg: colors.surfaceMuted,
        rowHoverBg: colors.primarySoft,
        borderColor: colors.border,
        headerColor: colors.textMuted,
      },
      Modal: {
        contentBg: colors.surface,
        headerBg: colors.surface,
      },
      Tabs: {
        itemSelectedColor: colors.primary,
        inkBarColor: colors.primary,
      },
      Tree: {
        nodeHoverBg: colors.primarySoft,
        nodeSelectedBg: colors.primarySoft,
      },
      Select: {
        optionSelectedBg: colors.primarySoft,
      },
      Dropdown: {
        controlItemBgHover: colors.primarySoft,
      },
    },
  }
}

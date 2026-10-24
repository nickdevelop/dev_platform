import { App as AntApp, ConfigProvider } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import { useEffect, type ReactNode } from 'react'
import { useAppearanceStore } from '../stores/appearance'
import { buildAntdTheme } from '../theme/antd'
import { applyAppearance } from '../theme/apply'

type ComponentSize = 'small' | 'middle' | 'large'

function componentSizeFor(density: string): ComponentSize {
  if (density === 'compact') return 'small'
  if (density === 'relaxed') return 'large'
  return 'middle'
}

export function AppProviders({ children }: { children: ReactNode }) {
  const appearance = useAppearanceStore((state) => state.appearance)

  // The CSS variables and the antd token object share presets.ts as their only
  // source. Applying here keeps both layers in sync on every theme/density
  // change, including the first render after the cached value was painted.
  useEffect(() => {
    applyAppearance(appearance)
  }, [appearance])

  return (
    <ConfigProvider
      locale={zhCN}
      componentSize={componentSizeFor(appearance.density)}
      theme={buildAntdTheme(appearance)}
    >
      <AntApp>{children}</AntApp>
    </ConfigProvider>
  )
}

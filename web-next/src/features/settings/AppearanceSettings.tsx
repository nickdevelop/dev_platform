import { Alert, Button, Divider, Input, Segmented, Slider, Switch, Typography } from 'antd'
import { useEffect, useState } from 'react'
import { useAppearanceStore } from '../../stores/appearance'
import { useSettingsStore } from '../../stores/settings'
import { densities, themes } from '../../theme/presets'
import styles from './AppearanceSettings.module.css'

export function AppearanceSettings() {
  const appearance = useAppearanceStore((state) => state.appearance)
  const setAppearance = useAppearanceStore((state) => state.setAppearance)
  const loadAppearance = useSettingsStore((state) => state.loadAppearance)
  const saveAppearance = useSettingsStore((state) => state.saveAppearance)
  const quickAssistant = useSettingsStore((state) => state.quickAssistant)
  const loadQuickAssistant = useSettingsStore((state) => state.loadQuickAssistant)
  const saveQuickAssistant = useSettingsStore((state) => state.saveQuickAssistantEnabled)
  const error = useSettingsStore((state) => state.error)
  const quickError = useSettingsStore((state) => state.quickAssistantError)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    void loadAppearance()
    void loadQuickAssistant()
  }, [loadAppearance, loadQuickAssistant])

  async function save(patch: Parameters<typeof setAppearance>[0]) {
    setAppearance(patch)
    setSaving(true)
    try {
      const ok = await saveAppearance(patch)
      if (!ok) setAppearance(appearance)
    } finally {
      setSaving(false)
    }
  }

  return (
    <section className={styles.section} aria-label="外观设置">
      <Typography.Title level={2}>外观</Typography.Title>

      <fieldset className={styles.group}>
        <legend>主题配色</legend>
        <div className={styles.themeGrid}>
          {themes.map((theme) => (
            <button
              key={theme.key}
              type="button"
              className={`${styles.theme} ${appearance.theme === theme.key ? styles.selected : ''}`}
              onClick={() => void save({ theme: theme.key })}
            >
              <span
                className={styles.swatch}
                style={{
                  background: theme.colors.bg,
                  borderColor: theme.colors.borderStrong,
                  color: theme.colors.text,
                }}
              >
                Aa
              </span>
              {theme.label}
            </button>
          ))}
        </div>
      </fieldset>

      <fieldset className={styles.group}>
        <legend>字号（{appearance.fontSize}px）</legend>
        <Slider
          min={12}
          max={20}
          step={1}
          value={appearance.fontSize}
          onChange={(value) => setAppearance({ fontSize: value })}
          onChangeComplete={(value) => void save({ fontSize: value })}
        />
      </fieldset>

      <fieldset className={styles.group}>
        <legend>界面密度</legend>
        <Segmented
          value={appearance.density}
          onChange={(value) => void save({ density: String(value) })}
          options={densities.map((density) => ({ label: density.label, value: density.key }))}
        />
      </fieldset>

      <fieldset className={styles.group}>
        <legend>PlantUML 渲染模式</legend>
        <Segmented
          value={appearance.plantumlMode ?? 'local'}
          onChange={(value) => void save({ plantumlMode: value as 'local' | 'online' })}
          options={[
            { label: '离线渲染（本地）', value: 'local' },
            { label: '在线渲染（服务器）', value: 'online' },
          ]}
        />
        {appearance.plantumlMode === 'online' && (
          <Input
            className={styles.url}
            defaultValue={appearance.plantumlServerUrl ?? 'https://www.plantuml.com/plantuml'}
            placeholder="PlantUML 服务地址"
            onBlur={(event) => void save({ plantumlServerUrl: event.target.value })}
          />
        )}
      </fieldset>

      <Divider />
      <fieldset className={styles.group}>
        <legend>快捷助手</legend>
        <div className={styles.switchRow}>
          <Switch
            aria-label="显示快捷助手漂浮按钮"
            checked={quickAssistant.enabled}
            onChange={async (checked) => {
              const ok = await saveQuickAssistant(checked)
              if (!ok) setAppearance(appearance)
            }}
          />
          <span>显示快捷助手漂浮按钮</span>
        </div>
        <Typography.Paragraph type="secondary">
          快捷助手产生的对话统一保存在 Chat 的“快捷助手”固定分组。
        </Typography.Paragraph>
      </fieldset>

      {saving && <Typography.Text type="secondary">保存中…</Typography.Text>}
      {(error !== '' || quickError !== '') && (
        <Alert type="error" showIcon title={error || quickError} />
      )}
      <Button onClick={() => void loadAppearance()}>重新读取外观</Button>
    </section>
  )
}

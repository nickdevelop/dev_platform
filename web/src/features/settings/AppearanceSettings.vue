<script setup lang="ts">
// Appearance settings section: theme palette and font size.
// Types of font face are built-in (Google Sans Code + Noto Sans SC).
// Every change applies immediately and syncs with the server.
//
// Element Plus owns the controls: the theme picker is an `el-radio-group` whose
// options carry a live swatch, the font tier is a three-way radio pair, and the
// quick-assistant float is an `el-switch` (the `sr-only peer` hand-rolled toggle
// it replaces had no focus ring and no aria role). Failures go through
// `ElMessage` once instead of stacking a `UiAlert` banner under the form.
import { computed, onMounted, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { ElMessage } from 'element-plus'
import { useSettingsStore } from '../../stores/settings'
import { fontSizes, resolveFontSizeKey, themes } from '../../theme/presets'

const settings = useSettingsStore()
const {
  appearance,
  quickAssistant,
  quickAssistantSaving,
  quickAssistantLoading,
  error,
  quickAssistantError,
} = storeToRefs(settings)
const DEFAULT_PLANTUML_SERVER_URL = 'https://www.plantuml.com/plantuml'

const PLANTUML_MODES = [
  { value: 'local', label: '离线渲染（本地）', desc: '浏览器本地编译，无数据外传' },
  { value: 'online', label: '在线渲染（服务器）', desc: '支持复杂图表与高吞吐渲染' },
] as const

onMounted(() => {
  void settings.loadAppearance()
  void settings.loadQuickAssistant()
})

// The store keeps the last good value on a failed save; the report belongs here
// rather than in a banner that pushes the rest of the page down (v2 §五).
watch(error, (message) => {
  if (message) ElMessage.error(message)
})
watch(quickAssistantError, (message) => {
  if (message) ElMessage.error(message)
})

// el-radio-group is driven by the store, so every option value has to be a
// real string the group can compare against; the font tier is a number.
const themeKey = computed(() => appearance.value.theme)
const fontSizeKey = computed(() => resolveFontSizeKey(appearance.value.fontSize))
const plantumlMode = computed(() => appearance.value.plantumlMode || 'local')

function onThemeChange(key: string | number | boolean | undefined) {
  if (String(key) !== appearance.value.theme) void settings.saveAppearance({ theme: String(key) })
}

function onFontSizeChange(key: string | number | boolean | undefined) {
  const opt = fontSizes.find((entry) => entry.key === String(key))
  if (!opt) return
  if (appearance.value.fontSize !== opt.value) void settings.saveAppearance({ fontSize: opt.value })
}

function onPlantumlModeChange(mode: string | number | boolean | undefined) {
  const next = String(mode) as 'local' | 'online'
  if (appearance.value.plantumlMode !== next) void settings.saveAppearance({ plantumlMode: next })
}

function onPlantumlServerUrlChange(value: string) {
  void settings.saveAppearance({ plantumlServerUrl: value.trim() })
}

async function onQuickAssistantChange(checked: boolean) {
  const ok = await settings.saveQuickAssistantEnabled(checked)
  if (!ok) {
    // The store did not move, so the control must not either: push the last
    // good value back in or the switch would sit showing a state that never
    // happened.
    ElMessage.error(settings.quickAssistantError || '保存快捷助手设置失败')
  }
}
</script>

<template>
  <section class="appearance-settings space-y-4" aria-label="外观设置">
    <header class="settings-page-head">
      <h1 class="settings-page-title">外观</h1>
      <p class="settings-page-desc">
        自定义工作台的主题配色、显示字号以及 PlantUML 架构图渲染方式
      </p>
    </header>

    <!-- Theme Palette -->
    <section class="settings-panel" aria-labelledby="appearance-theme-title">
      <header class="settings-panel-head">
        <h2 id="appearance-theme-title" class="settings-panel-title">主题配色</h2>
        <p class="settings-panel-desc">选择符合您视觉习惯的配色体系，支持明亮浅色与低眩光暖冷色系</p>
      </header>

      <el-radio-group
        class="theme-grid flex flex-wrap gap-2"
        :model-value="themeKey"
        @change="onThemeChange"
      >
        <el-radio
          v-for="theme in themes"
          :key="theme.key"
          :value="theme.key"
          border
          class="option-card theme-card"
          :data-theme-option="theme.key"
        >
          <div class="theme-body">
            <!-- Live swatch preview: the palette itself, not a decoration. -->
            <div
              class="swatch-preview"
              :style="{
                background: theme.previewGradient || theme.colors.bg,
                borderColor: theme.colors.borderStrong,
              }"
            >
              <div class="swatch-top">
                <span
                  class="swatch-glyph"
                  :style="{ color: theme.colors.text, background: theme.colors.surface }"
                >
                  Aa
                </span>
                <span class="swatch-dot" :style="{ background: theme.colors.primary }"></span>
              </div>
              <span class="swatch-rule" :style="{ background: theme.colors.border }"></span>
            </div>
            <span class="theme-label">{{ theme.label }}</span>
          </div>
        </el-radio>
      </el-radio-group>
    </section>

    <!-- Font Size -->
    <section class="settings-panel" aria-labelledby="appearance-font-title">
      <header class="settings-panel-head">
        <h2 id="appearance-font-title" class="settings-panel-title">字号</h2>
        <p class="settings-panel-desc">控制全平台界面字体基准大小，各区域文本会按比例缩放</p>
      </header>

      <el-radio-group
        class="font-size-row w-full"
        data-font-size
        :model-value="fontSizeKey"
        @change="onFontSizeChange"
      >
        <el-radio
          v-for="opt in fontSizes"
          :key="opt.key"
          :value="opt.key"
          border
          class="option-card font-size-option"
          :data-font-size-option="opt.key"
        >
          {{ opt.label }}
        </el-radio>
      </el-radio-group>
    </section>

    <!-- PlantUML Rendering Mode -->
    <section class="settings-panel" aria-labelledby="appearance-plantuml-title">
      <header class="settings-panel-head">
        <h2 id="appearance-plantuml-title" class="settings-panel-title">PlantUML 渲染模式</h2>
        <p class="settings-panel-desc">
          对话或文档中的时序图与架构图渲染引擎设置，支持内建本地离线生成或私有服务器
        </p>
      </header>

      <el-radio-group
        class="mode-row w-full"
        :model-value="plantumlMode"
        @change="onPlantumlModeChange"
      >
        <el-radio
          v-for="mode in PLANTUML_MODES"
          :key="mode.value"
          :value="mode.value"
          border
          class="option-card mode-option"
          :data-plantuml-mode="mode.value"
        >
          <div class="mode-body">
            <span class="mode-label">{{ mode.label }}</span>
            <span class="mode-desc">{{ mode.desc }}</span>
          </div>
        </el-radio>
      </el-radio-group>

      <div
        v-if="appearance.plantumlMode === 'online'"
        class="plantuml-url-field pt-1"
      >
        <el-input
          data-plantuml-url
          :model-value="appearance.plantumlServerUrl || DEFAULT_PLANTUML_SERVER_URL"
          placeholder="https://www.plantuml.com/plantuml"
          @change="onPlantumlServerUrlChange"
        />
        <p class="settings-field-hint">
          支持填入企业内网自建 PlantUML 服务地址，默认使用官方安全渲染端点
        </p>
      </div>
    </section>

    <!-- Quick Assistant Switch -->
    <section class="settings-panel" aria-labelledby="appearance-quick-title">
      <header class="settings-panel-head">
        <h2 id="appearance-quick-title" class="settings-panel-title">快捷助手</h2>
      </header>

      <div class="settings-switch-row">
        <div class="min-w-0">
          <div class="settings-switch-title">显示快捷助手漂浮按钮</div>
          <p class="settings-switch-desc">
            开启后，平台右上角将常驻快捷悬浮球，支持随时唤起 AI 提问；对话历史自动同步至 Chat「快捷助手」分组
          </p>
        </div>

        <el-switch
          class="shrink-0"
          :model-value="quickAssistant.enabled"
          :disabled="quickAssistantSaving || quickAssistantLoading"
          aria-label="显示快捷助手漂浮按钮"
          data-test="quick-assistant-toggle"
          @change="onQuickAssistantChange($event as boolean)"
        />
      </div>
    </section>
  </section>
</template>

<style scoped>
/* Only the theme cards' own content lives here: the radio's border, radius,
   checked edge and the card's own box are the library's (plus the `.option-card`
   bridge in the token layer). */
.theme-card {
  margin-right: 0;
}

.theme-body {
  display: flex;
  flex-direction: column;
  gap: 6px;
  width: 132px;
}

.swatch-preview {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  gap: 6px;
  height: 44px;
  padding: 6px;
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
}

.swatch-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.swatch-glyph {
  padding: 0 3px;
  border-radius: 2px;
  font-size: var(--text-size-11);
  font-weight: 600;
}

.swatch-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex: none;
}

.swatch-rule {
  display: block;
  width: 100%;
  height: 2px;
  border-radius: 1px;
  opacity: 0.85;
}

.theme-label {
  font-size: var(--text-size-12);
  font-weight: 500;
}

.font-size-option {
  font-size: var(--text-size-12);
}

.font-size-row,
.mode-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.font-size-option {
  margin-right: 0;
  flex: 1 1 90px;
}

.mode-row {
  align-items: stretch;
}

.mode-option {
  margin-right: 0;
  flex: 1 1 150px;
}

.mode-body {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.mode-label {
  font-size: var(--text-size-12);
  font-weight: 500;
}

.mode-desc {
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
}

.plantuml-url-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  border-top: 1px solid var(--color-border);
}
</style>

<script setup lang="ts">
// Appearance settings section: theme palette and font size.
// Types of font face are built-in (Google Sans Code + Noto Sans SC).
// Every change applies immediately and syncs with the server.
// Redesigned with design-taste-frontend app-screens guidelines (framed-sheet,
// flat surfaces, zero resting shadows, precise tactile swatches, accessible controls).
import { onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { useSettingsStore } from '../../stores/settings'
import { fontSizes, resolveFontSizeKey, themes } from '../../theme/presets'
import { UiAlert, UiInput } from '../../shared/ui'

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

onMounted(() => {
  void settings.loadAppearance()
  void settings.loadQuickAssistant()
})

function selectTheme(key: string) {
  if (settings.appearance.theme !== key) void settings.saveAppearance({ theme: key })
}

function selectFontSize(value: number) {
  if (settings.appearance.fontSize !== value) void settings.saveAppearance({ fontSize: value })
}

function selectPlantumlMode(mode: 'local' | 'online') {
  if (settings.appearance.plantumlMode !== mode) void settings.saveAppearance({ plantumlMode: mode })
}

function updatePlantumlServerUrl(value: string) {
  void settings.saveAppearance({ plantumlServerUrl: value.trim() })
}

function onPlantumlServerUrlChange(e: Event) {
  const input = e.target as HTMLInputElement
  updatePlantumlServerUrl(input.value)
}

async function toggleQuickAssistant(event: Event) {
  const input = event.target as HTMLInputElement
  const ok = await settings.saveQuickAssistantEnabled(input.checked)
  if (!ok) input.checked = settings.quickAssistant.enabled
}
</script>

<template>
  <section class="appearance-settings w-full space-y-6" aria-label="外观设置">
    <!-- Header -->
    <header class="settings-head pb-4 border-b border-platform-border">
      <h1 class="text-2xl font-semibold text-platform-text tracking-tight">外观</h1>
      <p class="text-xs text-platform-text-muted mt-1">
        自定义工作台的主题配色、显示字号以及 PlantUML 架构图渲染方式。
      </p>
    </header>

    <!-- Theme Palette -->
    <fieldset class="setting-group p-5 rounded-lg bg-platform-surface border border-platform-border space-y-4">
      <div class="group-header">
        <legend class="text-sm font-semibold text-platform-text px-0">主题配色</legend>
        <p class="text-xs text-platform-text-muted mt-0.5">选择符合您视觉习惯的配色体系，支持明亮浅色与低眩光暖冷色系。</p>
      </div>

      <div class="theme-grid grid grid-cols-1 sm:grid-cols-3 gap-3">
        <button
          v-for="theme in themes"
          :key="theme.key"
          type="button"
          class="theme-card relative flex flex-col p-3 rounded-lg border text-left cursor-pointer transition-all duration-150 select-none active:scale-[0.98] focus-visible:outline-2 focus-visible:outline-platform-primary focus-visible:outline-offset-2"
          :class="[
            appearance.theme === theme.key
              ? 'border-platform-primary bg-platform-primary-soft text-platform-text shadow-sm'
              : 'border-platform-border hover:border-platform-border-strong bg-platform-surface-muted/50 text-platform-text-muted hover:text-platform-text',
          ]"
          :data-theme-option="theme.key"
          @click="selectTheme(theme.key)"
        >
          <!-- Live swatch preview -->
          <div
            class="swatch-preview w-full h-14 rounded-md border flex flex-col justify-between p-2 mb-2.5 overflow-hidden shadow-inner"
            :style="{
              background: theme.colors.bg,
              borderColor: theme.colors.borderStrong,
            }"
          >
            <div class="flex items-center justify-between">
              <span
                class="text-xs font-semibold px-1 rounded"
                :style="{ color: theme.colors.text, background: theme.colors.surface }"
              >
                Aa
              </span>
              <span
                class="w-2.5 h-2.5 rounded-full"
                :style="{ background: theme.colors.primary }"
              ></span>
            </div>
            <div
              class="w-full h-2 rounded-sm opacity-80"
              :style="{ background: theme.colors.border }"
            ></div>
          </div>

          <div class="flex items-center justify-between">
            <span class="text-xs font-medium">{{ theme.label }}</span>
            <span
              v-if="appearance.theme === theme.key"
              class="w-2 h-2 rounded-full bg-platform-primary"
            ></span>
          </div>
        </button>
      </div>
    </fieldset>

    <!-- Font Size -->
    <fieldset class="setting-group p-5 rounded-lg bg-platform-surface border border-platform-border space-y-4">
      <div class="group-header">
        <legend class="text-sm font-semibold text-platform-text px-0">字号</legend>
        <p class="text-xs text-platform-text-muted mt-0.5">控制全平台界面字体基准大小，各区域文本会按比例缩放。</p>
      </div>

      <div class="font-size-row flex flex-wrap gap-2.5" data-font-size>
        <button
          v-for="opt in fontSizes"
          :key="opt.key"
          type="button"
          class="font-size-option flex-1 min-w-[90px] py-2.5 px-3 rounded-md border text-center transition-all duration-150 select-none active:scale-[0.98] focus-visible:outline-2 focus-visible:outline-platform-primary focus-visible:outline-offset-2"
          :class="[
            resolveFontSizeKey(appearance.fontSize) === opt.key
              ? 'border-platform-primary bg-platform-primary-soft text-platform-primary font-medium shadow-sm'
              : 'border-platform-border hover:border-platform-border-strong text-platform-text-muted hover:text-platform-text bg-platform-surface',
          ]"
          :data-font-size-option="opt.key"
          @click="selectFontSize(opt.value)"
        >
          <div class="text-xs">{{ opt.label }}</div>
        </button>
      </div>
    </fieldset>

    <!-- PlantUML Rendering Mode -->
    <fieldset class="setting-group p-5 rounded-lg bg-platform-surface border border-platform-border space-y-4">
      <div class="group-header">
        <legend class="text-sm font-semibold text-platform-text px-0">PlantUML 渲染模式</legend>
        <p class="text-xs text-platform-text-muted mt-0.5">对话或文档中的时序图与架构图渲染引擎设置，支持内建本地离线生成或私有服务器。</p>
      </div>

      <div class="mode-row flex flex-wrap gap-2.5">
        <button
          type="button"
          class="mode-option flex-1 min-w-[120px] py-2.5 px-3 rounded-md border text-center transition-all duration-150 select-none active:scale-[0.98] focus-visible:outline-2 focus-visible:outline-platform-primary focus-visible:outline-offset-2"
          :class="[
            (appearance.plantumlMode || 'local') === 'local'
              ? 'border-platform-primary bg-platform-primary-soft text-platform-primary font-medium shadow-sm'
              : 'border-platform-border hover:border-platform-border-strong text-platform-text-muted hover:text-platform-text bg-platform-surface',
          ]"
          data-plantuml-mode="local"
          @click="selectPlantumlMode('local')"
        >
          <div class="text-xs font-medium">离线渲染（本地）</div>
          <div class="text-2xs text-platform-text-muted mt-0.5">浏览器本地编译，无数据外传</div>
        </button>
        <button
          type="button"
          class="mode-option flex-1 min-w-[120px] py-2.5 px-3 rounded-md border text-center transition-all duration-150 select-none active:scale-[0.98] focus-visible:outline-2 focus-visible:outline-platform-primary focus-visible:outline-offset-2"
          :class="[
            appearance.plantumlMode === 'online'
              ? 'border-platform-primary bg-platform-primary-soft text-platform-primary font-medium shadow-sm'
              : 'border-platform-border hover:border-platform-border-strong text-platform-text-muted hover:text-platform-text bg-platform-surface',
          ]"
          data-plantuml-mode="online"
          @click="selectPlantumlMode('online')"
        >
          <div class="text-xs font-medium">在线渲染（服务器）</div>
          <div class="text-2xs text-platform-text-muted mt-0.5">支持复杂图表与高吞吐渲染</div>
        </button>
      </div>

      <!-- Online mode server URL input -->
      <div v-if="appearance.plantumlMode === 'online'" class="plantuml-url-field pt-2 space-y-1.5 border-t border-platform-border">
        <label class="field flex flex-col gap-1 text-xs font-medium text-platform-text">
          <span>服务器地址</span>
          <UiInput
            data-plantuml-url
            class="plantuml-url-input text-xs"
            placeholder="https://www.plantuml.com/plantuml"
            :model-value="appearance.plantumlServerUrl || DEFAULT_PLANTUML_SERVER_URL"
            block
            @change="onPlantumlServerUrlChange"
          />
        </label>
        <span class="field-hint text-2xs text-platform-text-muted">
          支持填入企业内网自建 PlantUML 服务地址，默认使用官方安全渲染端点。
        </span>
      </div>
    </fieldset>

    <!-- Quick Assistant Switch -->
    <fieldset class="setting-group p-5 rounded-lg bg-platform-surface border border-platform-border space-y-3">
      <div class="group-header">
        <legend class="text-sm font-semibold text-platform-text px-0">快捷助手</legend>
      </div>

      <div class="flex items-center justify-between gap-4 py-1">
        <div class="space-y-0.5">
          <div class="text-xs font-medium text-platform-text">显示快捷助手漂浮按钮</div>
          <p class="text-2xs text-platform-text-muted leading-relaxed max-w-lg">
            开启后，平台右上角将常驻快捷悬浮球，支持随时唤起 AI 提问；对话历史自动同步至 Chat「快捷助手」分组。
          </p>
        </div>

        <label class="toggle-control relative inline-flex items-center cursor-pointer select-none shrink-0">
          <input
            type="checkbox"
            data-test="quick-assistant-toggle"
            class="sr-only peer"
            :checked="quickAssistant.enabled"
            :disabled="quickAssistantSaving || quickAssistantLoading"
            @change="toggleQuickAssistant"
          />
          <div class="w-9 h-5 bg-platform-border-strong peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-platform-border after:border after:rounded-full after:h-4 after:w-4 after:transition-transform peer-checked:bg-platform-primary relative"></div>
        </label>
      </div>
    </fieldset>

    <!-- Error alerts -->
    <UiAlert v-if="error" variant="error" class="error">
      {{ error }}
    </UiAlert>
    <UiAlert v-if="quickAssistantError" variant="error" class="error">
      {{ quickAssistantError }}
    </UiAlert>
  </section>
</template>

<style scoped>
.setting-group {
  margin: 0;
}
</style>

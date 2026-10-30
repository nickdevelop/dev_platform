<script setup lang="ts">
// Appearance settings section: theme palette, font size and interface density.
// Types of font face are built-in (Google Sans Code + Noto Sans SC).
// Every change applies immediately and syncs with the server.
// Redesigned with design-taste-frontend app-screens guidelines (framed-sheet,
// flat surfaces, zero resting shadows, precise tactile swatches, accessible controls).
import { onMounted } from 'vue'
import { useSettingsStore } from '../../stores/settings'
import { densities, fontSizes, resolveFontSizeKey, themes } from '../../theme/presets'
import { UiAlert, UiInput } from '../../shared/ui'

const settings = useSettingsStore()

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

function selectDensity(key: string) {
  if (settings.appearance.density !== key) void settings.saveAppearance({ density: key })
}

function selectPlantumlMode(mode: 'local' | 'online') {
  if (settings.appearance.plantumlMode !== mode) void settings.saveAppearance({ plantumlMode: mode })
}

function updatePlantumlServerUrl(value: string) {
  void settings.saveAppearance({ plantumlServerUrl: value.trim() })
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
    <header class="settings-head pb-4 border-b border-slate-200 dark:border-slate-800">
      <h1 class="text-[22px] font-semibold text-slate-900 dark:text-slate-100 tracking-tight">外观</h1>
      <p class="text-xs text-slate-500 dark:text-slate-400 mt-1">
        自定义工作台的主题配色、显示字号、界面密度以及 PlantUML 架构图渲染方式。
      </p>
    </header>

    <!-- Theme Palette -->
    <fieldset class="setting-group p-5 rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 space-y-4">
      <div class="group-header">
        <legend class="text-sm font-semibold text-slate-800 dark:text-slate-200 px-0">主题配色</legend>
        <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">选择符合您视觉习惯的配色体系，支持明亮浅色、极简深色与低眩光暖冷色系。</p>
      </div>

      <div class="theme-grid grid grid-cols-2 sm:grid-cols-4 gap-3">
        <button
          v-for="theme in themes"
          :key="theme.key"
          type="button"
          class="theme-card relative flex flex-col p-3 rounded-md border text-left cursor-pointer transition-colors select-none focus-visible:outline-2 focus-visible:outline-blue-600 focus-visible:outline-offset-2"
          :class="[
            settings.appearance.theme === theme.key
              ? 'border-blue-600 bg-blue-50/20 dark:bg-blue-950/20 text-slate-900 dark:text-slate-100'
              : 'border-slate-200 dark:border-slate-700/80 hover:border-slate-300 dark:hover:border-slate-600 bg-slate-50/50 dark:bg-slate-800/30 text-slate-700 dark:text-slate-300',
          ]"
          :data-theme-option="theme.key"
          @click="selectTheme(theme.key)"
        >
          <!-- Live swatch preview -->
          <div
            class="swatch-preview w-full h-14 rounded border flex flex-col justify-between p-2 mb-2.5 overflow-hidden"
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
                class="w-2 h-2 rounded-full"
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
              v-if="settings.appearance.theme === theme.key"
              class="w-1.5 h-1.5 rounded-full bg-blue-600 dark:bg-blue-400"
            ></span>
          </div>
        </button>
      </div>
    </fieldset>

    <!-- Font Size -->
    <fieldset class="setting-group p-5 rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 space-y-4">
      <div class="group-header">
        <legend class="text-sm font-semibold text-slate-800 dark:text-slate-200 px-0">字号</legend>
        <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">控制全平台界面字体基准大小，代码区与对话区会自适应成比例缩放。</p>
      </div>

      <div class="font-size-row flex flex-wrap gap-2.5" data-font-size>
        <button
          v-for="opt in fontSizes"
          :key="opt.key"
          type="button"
          class="font-size-option flex-1 min-w-[90px] py-2.5 px-3 rounded-md border text-center transition-colors select-none focus-visible:outline-2 focus-visible:outline-blue-600 focus-visible:outline-offset-2"
          :class="[
            resolveFontSizeKey(settings.appearance.fontSize) === opt.key
              ? 'border-blue-600 bg-blue-50/20 dark:bg-blue-950/20 text-blue-700 dark:text-blue-300 font-medium'
              : 'border-slate-200 dark:border-slate-700 hover:border-slate-300 dark:hover:border-slate-600 text-slate-700 dark:text-slate-300',
          ]"
          :data-font-size-option="opt.key"
          @click="selectFontSize(opt.value)"
        >
          <div class="text-xs mb-0.5">{{ opt.label }}</div>
          <div class="text-[11px] tabular-nums text-slate-400 dark:text-slate-500">{{ opt.value }}px</div>
        </button>
      </div>
    </fieldset>

    <!-- Density -->
    <fieldset class="setting-group p-5 rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 space-y-4">
      <div class="group-header">
        <legend class="text-sm font-semibold text-slate-800 dark:text-slate-200 px-0">界面密度</legend>
        <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">调整工作台边距、列表高度与控件尺寸，紧凑模式利于高信息密度作业。</p>
      </div>

      <div class="density-row flex flex-wrap gap-2.5">
        <button
          v-for="density in densities"
          :key="density.key"
          type="button"
          class="density-option flex-1 min-w-[90px] py-2.5 px-3 rounded-md border text-center transition-colors select-none focus-visible:outline-2 focus-visible:outline-blue-600 focus-visible:outline-offset-2"
          :class="[
            settings.appearance.density === density.key
              ? 'border-blue-600 bg-blue-50/20 dark:bg-blue-950/20 text-blue-700 dark:text-blue-300 font-medium'
              : 'border-slate-200 dark:border-slate-700 hover:border-slate-300 dark:hover:border-slate-600 text-slate-700 dark:text-slate-300',
          ]"
          :data-density-option="density.key"
          @click="selectDensity(density.key)"
        >
          <div class="text-xs mb-0.5">{{ density.label }}</div>
          <div class="text-[11px] tabular-nums text-slate-400 dark:text-slate-500">{{ density.controlHeight }}</div>
        </button>
      </div>
    </fieldset>

    <!-- PlantUML Rendering Mode -->
    <fieldset class="setting-group p-5 rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 space-y-4">
      <div class="group-header">
        <legend class="text-sm font-semibold text-slate-800 dark:text-slate-200 px-0">PlantUML 渲染模式</legend>
        <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">对话或文档中的时序图与架构图渲染引擎设置，支持内建本地离线生成或私有服务器。</p>
      </div>

      <div class="density-row flex flex-wrap gap-2.5">
        <button
          type="button"
          class="density-option flex-1 min-w-[120px] py-2.5 px-3 rounded-md border text-center transition-colors select-none focus-visible:outline-2 focus-visible:outline-blue-600 focus-visible:outline-offset-2"
          :class="[
            (settings.appearance.plantumlMode || 'local') === 'local'
              ? 'border-blue-600 bg-blue-50/20 dark:bg-blue-950/20 text-blue-700 dark:text-blue-300 font-medium'
              : 'border-slate-200 dark:border-slate-700 hover:border-slate-300 dark:hover:border-slate-600 text-slate-700 dark:text-slate-300',
          ]"
          data-plantuml-mode="local"
          @click="selectPlantumlMode('local')"
        >
          <div class="text-xs font-medium">离线渲染（本地）</div>
          <div class="text-[11px] text-slate-400 mt-0.5">浏览器本地编译，无数据外传</div>
        </button>
        <button
          type="button"
          class="density-option flex-1 min-w-[120px] py-2.5 px-3 rounded-md border text-center transition-colors select-none focus-visible:outline-2 focus-visible:outline-blue-600 focus-visible:outline-offset-2"
          :class="[
            settings.appearance.plantumlMode === 'online'
              ? 'border-blue-600 bg-blue-50/20 dark:bg-blue-950/20 text-blue-700 dark:text-blue-300 font-medium'
              : 'border-slate-200 dark:border-slate-700 hover:border-slate-300 dark:hover:border-slate-600 text-slate-700 dark:text-slate-300',
          ]"
          data-plantuml-mode="online"
          @click="selectPlantumlMode('online')"
        >
          <div class="text-xs font-medium">在线渲染（服务器）</div>
          <div class="text-[11px] text-slate-400 mt-0.5">支持复杂图表与高吞吐渲染</div>
        </button>
      </div>

      <!-- Online mode server URL input -->
      <div v-if="settings.appearance.plantumlMode === 'online'" class="plantuml-url-field pt-2 space-y-1.5 border-t border-slate-100 dark:border-slate-800">
        <label class="field flex flex-col gap-1 text-xs font-medium text-slate-700 dark:text-slate-300">
          <span>服务器地址</span>
          <UiInput
            data-plantuml-url
            class="plantuml-url-input text-xs"
            placeholder="https://www.plantuml.com/plantuml"
            :model-value="settings.appearance.plantumlServerUrl || 'https://www.plantuml.com/plantuml'"
            block
            @change="(e: Event) => updatePlantumlServerUrl((e.target as HTMLInputElement).value)"
          />
        </label>
        <span class="field-hint text-[11px] text-slate-400 dark:text-slate-500">
          支持填入企业内网自建 PlantUML 服务地址，默认使用官方安全渲染端点。
        </span>
      </div>
    </fieldset>

    <!-- Quick Assistant Switch -->
    <fieldset class="setting-group p-5 rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 space-y-3">
      <div class="group-header">
        <legend class="text-sm font-semibold text-slate-800 dark:text-slate-200 px-0">快捷助手</legend>
      </div>

      <div class="flex items-center justify-between gap-4 py-1">
        <div class="space-y-0.5">
          <div class="text-xs font-medium text-slate-800 dark:text-slate-200">显示快捷助手漂浮按钮</div>
          <p class="text-[11px] text-slate-500 dark:text-slate-400 leading-relaxed max-w-lg">
            开启后，平台右上角将常驻快捷悬浮球，支持随时唤起 AI 提问；对话历史自动同步至 Chat「快捷助手」分组。
          </p>
        </div>

        <label class="toggle-control relative inline-flex items-center cursor-pointer select-none shrink-0">
          <input
            type="checkbox"
            data-test="quick-assistant-toggle"
            class="sr-only peer"
            :checked="settings.quickAssistant.enabled"
            :disabled="settings.quickAssistantSaving || settings.quickAssistantLoading"
            @change="toggleQuickAssistant"
          />
          <div class="w-9 h-5 bg-slate-200 peer-focus:outline-none rounded-full peer dark:bg-slate-700 peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-transform dark:border-slate-600 peer-checked:bg-blue-600 relative"></div>
        </label>
      </div>
    </fieldset>

    <!-- Error alerts -->
    <UiAlert v-if="settings.error" variant="error" class="error">
      {{ settings.error }}
    </UiAlert>
    <UiAlert v-if="settings.quickAssistantError" variant="error" class="error">
      {{ settings.quickAssistantError }}
    </UiAlert>
  </section>
</template>

<style scoped>
.setting-group {
  margin: 0;
}
</style>

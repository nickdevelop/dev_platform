<script setup lang="ts">
// Appearance settings section: theme palette, font size and interface density.
// The type of face is not offered — the UI bundles Google Sans Code and Noto
// Sans SC and always uses them. Every change saves immediately and applies to
// the current page; the shared config means other open pages pick it up on
// refresh.
import { onMounted } from 'vue'
import { useSettingsStore } from '../../stores/settings'
import { densities, themes } from '../../theme/presets'
import { UiAlert, UiInput } from '../../shared/ui'

const settings = useSettingsStore()

onMounted(() => {
  void settings.loadAppearance()
  void settings.loadQuickAssistant()
})

function selectTheme(key: string) {
  if (settings.appearance.theme !== key) void settings.saveAppearance({ theme: key })
}

function updateFontSize(event: Event) {
  const value = Number((event.target as HTMLInputElement).value)
  if (Number.isFinite(value)) void settings.saveAppearance({ fontSize: value })
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
  <section class="appearance max-w-3xl space-y-5" aria-label="外观设置">
    <h1 class="text-xl font-bold text-slate-800 dark:text-slate-100 mb-4">外观</h1>

    <fieldset class="group p-4 rounded bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 space-y-3">
      <legend class="px-1 text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400">主题配色</legend>
      <div class="theme-grid grid grid-cols-2 sm:grid-cols-4 gap-3">
        <button
          v-for="theme in themes"
          :key="theme.key"
          type="button"
          class="theme-option flex items-center gap-3 p-3 rounded border text-sm font-medium transition-colors select-none text-left"
          :class="[
            settings.appearance.theme === theme.key
              ? 'border-blue-600 ring-1 ring-blue-600 bg-blue-50/30 dark:bg-blue-950/20 text-blue-700 dark:text-blue-300'
              : 'border-slate-300 dark:border-slate-700 hover:bg-slate-50 dark:hover:bg-slate-800 text-slate-700 dark:text-slate-200',
          ]"
          :data-theme-option="theme.key"
          @click="selectTheme(theme.key)"
        >
          <span
            class="swatch inline-flex items-center justify-center w-8 h-8 rounded border font-bold text-sm shrink-0"
            :style="{
              background: theme.colors.bg,
              borderColor: theme.colors.borderStrong,
              color: theme.colors.text,
            }"
          >Aa</span>
          <span class="truncate">{{ theme.label }}</span>
        </button>
      </div>
    </fieldset>

    <fieldset class="group p-4 rounded bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 space-y-3">
      <legend class="px-1 text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400">字号</legend>
      <label class="field flex flex-col gap-2 max-w-sm">
        <span class="text-sm text-slate-700 dark:text-slate-300">字号（{{ settings.appearance.fontSize }}px）</span>
        <input
          type="range"
          data-font-size
          min="12"
          max="20"
          step="1"
          class="accent-blue-600 cursor-pointer"
          :value="settings.appearance.fontSize"
          @change="updateFontSize"
        />
      </label>
    </fieldset>

    <fieldset class="group p-4 rounded bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 space-y-3">
      <legend class="px-1 text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400">界面密度</legend>
      <div class="density-row flex flex-wrap gap-3">
        <button
          v-for="density in densities"
          :key="density.key"
          type="button"
          class="density-option px-4 py-2 rounded border text-sm font-medium transition-colors select-none"
          :class="[
            settings.appearance.density === density.key
              ? 'border-blue-600 ring-1 ring-blue-600 bg-blue-50/30 dark:bg-blue-950/20 text-blue-700 dark:text-blue-300'
              : 'border-slate-300 dark:border-slate-700 hover:bg-slate-50 dark:hover:bg-slate-800 text-slate-700 dark:text-slate-200',
          ]"
          :data-density-option="density.key"
          @click="selectDensity(density.key)"
        >
          {{ density.label }}
        </button>
      </div>
    </fieldset>

    <fieldset class="group p-4 rounded bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 space-y-3">
      <legend class="px-1 text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400">PlantUML 渲染模式</legend>
      <div class="density-row flex flex-wrap gap-3">
        <button
          type="button"
          class="density-option px-4 py-2 rounded border text-sm font-medium transition-colors select-none"
          :class="[
            (settings.appearance.plantumlMode || 'local') === 'local'
              ? 'border-blue-600 ring-1 ring-blue-600 bg-blue-50/30 dark:bg-blue-950/20 text-blue-700 dark:text-blue-300'
              : 'border-slate-300 dark:border-slate-700 hover:bg-slate-50 dark:hover:bg-slate-800 text-slate-700 dark:text-slate-200',
          ]"
          data-plantuml-mode="local"
          @click="selectPlantumlMode('local')"
        >
          离线渲染（本地）
        </button>
        <button
          type="button"
          class="density-option px-4 py-2 rounded border text-sm font-medium transition-colors select-none"
          :class="[
            settings.appearance.plantumlMode === 'online'
              ? 'border-blue-600 ring-1 ring-blue-600 bg-blue-50/30 dark:bg-blue-950/20 text-blue-700 dark:text-blue-300'
              : 'border-slate-300 dark:border-slate-700 hover:bg-slate-50 dark:hover:bg-slate-800 text-slate-700 dark:text-slate-200',
          ]"
          data-plantuml-mode="online"
          @click="selectPlantumlMode('online')"
        >
          在线渲染（服务器）
        </button>
      </div>

      <div v-if="settings.appearance.plantumlMode === 'online'" class="plantuml-url-field pt-2 space-y-1.5">
        <label class="field flex flex-col gap-1 text-sm text-slate-700 dark:text-slate-300">
          <span>服务器地址</span>
          <UiInput
            data-plantuml-url
            class="plantuml-url-input"
            placeholder="https://www.plantuml.com/plantuml"
            :model-value="settings.appearance.plantumlServerUrl || 'https://www.plantuml.com/plantuml'"
            block
            @change="(e: Event) => updatePlantumlServerUrl((e.target as HTMLInputElement).value)"
          />
        </label>
        <span class="field-hint text-xs text-slate-400 dark:text-slate-500">支持自建 PlantUML 服务地址，默认使用官方服务器。</span>
      </div>
    </fieldset>

    <fieldset class="group p-4 rounded bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 space-y-3">
      <legend class="px-1 text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400">快捷助手</legend>
      <label class="toggle-row inline-flex items-center gap-2.5 text-sm text-slate-700 dark:text-slate-300 cursor-pointer select-none">
        <input
          type="checkbox"
          data-test="quick-assistant-toggle"
          class="w-4 h-4 rounded text-blue-600 focus:ring-blue-500 accent-blue-600 cursor-pointer disabled:cursor-wait"
          :checked="settings.quickAssistant.enabled"
          :disabled="settings.quickAssistantSaving || settings.quickAssistantLoading"
          @change="toggleQuickAssistant"
        />
        <span>显示快捷助手漂浮按钮</span>
      </label>
      <p class="field-hint text-xs text-slate-400 dark:text-slate-500 leading-relaxed">
        开启后，平台主界面右上角会显示快捷助手按钮；关闭后按钮与弹窗立即隐藏。
        快捷助手产生的对话统一保存在 Chat 的“快捷助手”固定分组。
      </p>
    </fieldset>

    <UiAlert v-if="settings.error" variant="error" class="error">
      {{ settings.error }}
    </UiAlert>
    <UiAlert v-if="settings.quickAssistantError" variant="error" class="error">
      {{ settings.quickAssistantError }}
    </UiAlert>
  </section>
</template>

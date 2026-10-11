<script setup lang="ts">
// Appearance settings section: theme, font (with a custom Google Fonts
// option), font size and interface density. Every change saves immediately and
// applies to the current page; the shared config means other open pages pick it
// up on refresh.
import { computed, onMounted } from 'vue'
import { useSettingsStore } from '../../stores/settings'
import { densities, fonts, themes } from '../../theme/presets'

const settings = useSettingsStore()

onMounted(() => {
  void settings.loadAppearance()
})

const fontSelection = computed(() => settings.appearance.font)

function selectTheme(key: string) {
  if (settings.appearance.theme !== key) void settings.saveAppearance({ theme: key })
}

function selectFont(event: Event) {
  const key = (event.target as HTMLSelectElement).value
  void settings.saveAppearance({ font: key })
}

function updateCustomFont(event: Event) {
  const value = (event.target as HTMLInputElement).value
  void settings.saveAppearance({ font: 'custom', customFont: value })
}

function updateFontSize(event: Event) {
  const value = Number((event.target as HTMLInputElement).value)
  if (Number.isFinite(value)) void settings.saveAppearance({ fontSize: value })
}

function selectDensity(key: string) {
  if (settings.appearance.density !== key) void settings.saveAppearance({ density: key })
}
</script>

<template>
  <section class="appearance" aria-label="外观设置">
    <h1>外观</h1>

    <fieldset class="group">
      <legend>主题配色</legend>
      <div class="theme-grid">
        <button
          v-for="theme in themes"
          :key="theme.key"
          type="button"
          class="theme-option"
          :class="{ active: settings.appearance.theme === theme.key }"
          :data-theme-option="theme.key"
          @click="selectTheme(theme.key)"
        >
          <span
            class="swatch"
            :style="{
              background: theme.colors.bg,
              borderColor: theme.colors.borderStrong,
              color: theme.colors.text,
            }"
            >Aa</span
          >
          {{ theme.label }}
        </button>
      </div>
    </fieldset>

    <fieldset class="group">
      <legend>字体</legend>
      <label class="field">
        <span>字体</span>
        <select data-font-select :value="fontSelection" @change="selectFont">
          <option v-for="font in fonts" :key="font.key" :value="font.key">{{ font.label }}</option>
          <option value="custom">自定义 Google Fonts</option>
        </select>
      </label>
      <label v-if="settings.appearance.font === 'custom'" class="field">
        <span>字体名称</span>
        <input
          type="text"
          data-custom-font
          placeholder="例如 Noto Sans SC"
          :value="settings.appearance.customFont"
          @change="updateCustomFont"
        />
      </label>
    </fieldset>

    <fieldset class="group">
      <legend>字号</legend>
      <label class="field">
        <span>字号（{{ settings.appearance.fontSize }}px）</span>
        <input
          type="range"
          data-font-size
          min="12"
          max="20"
          step="1"
          :value="settings.appearance.fontSize"
          @change="updateFontSize"
        />
      </label>
    </fieldset>

    <fieldset class="group">
      <legend>界面密度</legend>
      <div class="density-row">
        <button
          v-for="density in densities"
          :key="density.key"
          type="button"
          class="density-option"
          :class="{ active: settings.appearance.density === density.key }"
          :data-density-option="density.key"
          @click="selectDensity(density.key)"
        >
          {{ density.label }}
        </button>
      </div>
    </fieldset>

    <p v-if="settings.error" class="error" role="alert">{{ settings.error }}</p>
  </section>
</template>

<style scoped>
.appearance h1 {
  font-size: 1.25rem;
  margin: 0 0 1rem;
}

.group {
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  padding: calc(var(--space-unit) * 2);
  margin: 0 0 calc(var(--space-unit) * 2);
  background: var(--color-surface);
}

.group legend {
  padding: 0 calc(var(--space-unit) * 0.5);
  color: var(--color-text-muted);
}

.theme-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(110px, 1fr));
  gap: calc(var(--space-unit) * 1.5);
}

.theme-option,
.density-option {
  display: flex;
  align-items: center;
  gap: calc(var(--space-unit) * 1.5);
  padding: calc(var(--space-unit) * 1.5);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  cursor: pointer;
  font: inherit;
}

.theme-option.active,
.density-option.active {
  border-color: var(--color-primary);
  box-shadow: inset 0 0 0 1px var(--color-primary);
}

.swatch {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 2rem;
  height: 2rem;
  border: 1px solid;
  border-radius: var(--radius);
  font-weight: 600;
}

.field {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 0.5);
  margin-bottom: calc(var(--space-unit) * 1.5);
}

.field select,
.field input[type='text'] {
  height: var(--control-height);
  padding: 0 calc(var(--space-unit) * 1.5);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
}

.density-row {
  display: flex;
  gap: calc(var(--space-unit) * 1.5);
}

.error {
  color: var(--color-danger);
  margin: 0;
}
</style>

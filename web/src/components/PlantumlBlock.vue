<script setup lang="ts">
// One PlantUML diagram inside an agent reply. The block is shaped like the
// Thinking and tool blocks next to it: a title row that says what is in here,
// and the drawing underneath. The drawing is the default — a diagram is the
// reason the agent drew it — and the source is one click away for the person
// who wants to copy or correct it.
//
// Two rules the rest of the file serves: a reply that is still arriving holds a
// half-written fence, and asking the engine for that every few tokens is both
// pointless (it errors on the missing @enduml) and noisy, so drawing waits for
// `ready`. And the toggle belongs to this block alone — a message with three
// diagrams toggles them one at a time.
import { computed, ref, watch } from 'vue'
import { renderDiagram } from '../plantuml/renderer'
import { isDarkTheme } from '../theme/presets'
import { useSettingsStore } from '../stores/settings'

const props = defineProps<{ source: string; ready: boolean }>()

const settings = useSettingsStore()
const dark = computed(() => isDarkTheme(settings.appearance.theme))

const mode = ref<'preview' | 'source'>('preview')
const state = ref<'waiting' | 'rendering' | 'ready' | 'failed'>('waiting')
const svg = ref('')
const reason = ref('')

// Which (source, palette) pair is currently on screen, so a re-render does not
// redraw the diagram that is already there.
let drawn = ''
let request = 0

const headLabel = computed(() => {
  if (state.value === 'waiting') return '等待完成…'
  if (state.value === 'rendering') return '正在生成图…'
  if (state.value === 'failed') return ''
  return mode.value === 'preview' ? '查看源码' : '返回预览'
})

async function draw(): Promise<void> {
  const token = ++request
  if (!props.ready) {
    state.value = 'waiting'
    return
  }

  const key = `${dark.value ? 'd' : 'l'}\u0000${props.source}`
  if (drawn === key && state.value === 'ready') return

  state.value = 'rendering'
  const result = await renderDiagram({ source: props.source, dark: dark.value })
  if (token !== request) return // a newer reply or palette change superseded this one

  if (result.ok) {
    svg.value = result.svg
    drawn = key
    reason.value = ''
    state.value = 'ready'
    mode.value = 'preview'
  } else {
    reason.value = result.reason
    state.value = 'failed'
    mode.value = 'source'
  }
}

function toggle(): void {
  // A failed block has nothing to preview: the source is what it shows.
  if (state.value !== 'ready') return
  mode.value = mode.value === 'preview' ? 'source' : 'preview'
}

watch(() => [props.ready, props.source, dark.value], draw, { immediate: true })
</script>

<template>
  <div class="plantuml-block" data-test="plantuml-block">
    <button
      type="button"
      class="plantuml-head"
      :aria-expanded="String(state === 'ready' && mode === 'preview')"
      @click="toggle"
    >
      <span class="plantuml-chev" aria-hidden="true">{{ state === 'ready' && mode === 'preview' ? '▾' : '▸' }}</span>
      <span class="plantuml-title">图表 · plantuml</span>
      <span v-if="headLabel" class="plantuml-mode">{{ headLabel }}</span>
    </button>

    <div
      v-if="state === 'waiting' || state === 'rendering'"
      class="plantuml-note"
      data-test="plantuml-waiting"
    >
      <template v-if="state === 'waiting'">图还没写完,等这条回复结束再画。</template>
      <template v-else>正在生成图…</template>
    </div>

    <p v-if="state === 'failed'" class="plantuml-error" data-test="plantuml-error">
      无法渲染:{{ reason }}
    </p>

    <div
      v-if="state === 'ready' && mode === 'preview'"
      class="plantuml-canvas"
      data-test="plantuml-canvas"
      v-html="svg"
    ></div>

    <pre
      v-if="state === 'failed' || mode === 'source'"
      class="plantuml-src"
      data-test="plantuml-source"
    >{{ source }}</pre>
  </div>
</template>

<style scoped>
.plantuml-block {
  margin: calc(var(--space-unit) * 2) 0;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  overflow: hidden;
}

.plantuml-head {
  display: flex;
  align-items: center;
  gap: var(--space-unit);
  width: 100%;
  padding: var(--space-unit) calc(var(--space-unit) * 1.5);
  border: 0;
  border-bottom: 1px solid var(--color-border);
  background: var(--color-surface-muted);
  color: var(--color-text);
  font: inherit;
  font-size: calc(var(--font-size-base) - 1px);
  text-align: left;
  cursor: pointer;
}

.plantuml-chev {
  color: var(--color-text-muted);
}

.plantuml-title {
  color: var(--color-text);
}

.plantuml-mode {
  margin-left: auto;
  color: var(--color-primary);
}

.plantuml-note {
  padding: calc(var(--space-unit) * 2);
  color: var(--color-text-muted);
}

.plantuml-error {
  margin: 0;
  padding: var(--space-unit) calc(var(--space-unit) * 1.5);
  border-bottom: 1px solid var(--color-danger-border);
  color: var(--color-danger);
  background: var(--color-surface-muted);
}

/* A long sequence diagram must not stretch the message it arrived in. */
.plantuml-canvas {
  padding: calc(var(--space-unit) * 2);
  background: var(--color-surface);
  max-height: 60vh;
  overflow: auto;
}

.plantuml-canvas :deep(svg) {
  max-width: 100%;
  height: auto;
}

.plantuml-src {
  margin: 0;
  padding: calc(var(--space-unit) * 2);
  background: var(--color-surface-muted);
  color: var(--color-text);
  font-family: var(--font-family);
  font-size: calc(var(--font-size-base) - 1px);
  line-height: 1.55;
  overflow: auto;
  white-space: pre;
}
</style>

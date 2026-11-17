<script setup lang="ts">
// One Mermaid diagram inside an agent reply or markdown preview.
// Shows the rendered diagram by default, with source toggle and zoomable modal.
import { computed, ref, watch } from 'vue'
import { renderMermaidDiagram } from '../../../mermaid/renderer'
import DiagramViewer from '../../../shared/components/DiagramViewer.vue'
import { isDarkTheme } from '../../../theme/presets'
import { useSettingsStore } from '../../../stores/settings'

const props = defineProps<{ source: string; ready: boolean; finished: boolean }>()

const settings = useSettingsStore()
const dark = computed(() => isDarkTheme(settings.appearance.theme))

const mode = ref<'preview' | 'source'>('preview')
const state = ref<'waiting' | 'rendering' | 'ready' | 'failed' | 'incomplete'>('waiting')
const svg = ref('')
const reason = ref('')

let drawn = ''
let request = 0
const openedSource = ref(false)

const headLabel = computed(() => {
  if (state.value === 'waiting') return '等待完成…'
  if (state.value === 'rendering') return '正在生成图…'
  if (state.value === 'incomplete') return '图没画完'
  if (state.value === 'failed') return ''
  return mode.value === 'preview' ? '查看源码' : '返回预览'
})

const note = computed(() => {
  if (state.value === 'waiting') return '图还没写完，等生成结束再绘制。'
  if (state.value === 'rendering') return '正在生成图…'
  return '图未闭合（回复在围栏结束前停止），以下是源码。'
})

async function draw(): Promise<void> {
  const token = ++request
  if (!props.ready) {
    state.value = props.finished ? 'incomplete' : 'waiting'
    return
  }

  const currentTheme = settings.appearance.theme || 'light'
  const key = `${currentTheme}\u0000${props.source}`
  if (drawn === key && state.value === 'ready') return

  state.value = 'rendering'
  const result = await renderMermaidDiagram({
    source: props.source,
    dark: dark.value,
    themeKey: currentTheme,
  })
  if (token !== request) return

  if (result.ok) {
    svg.value = result.svg
    drawn = key
    reason.value = ''
    state.value = 'ready'
    if (!openedSource.value) mode.value = 'preview'
  } else {
    reason.value = result.reason
    state.value = 'failed'
    mode.value = 'source'
  }
}

function toggle(): void {
  if (state.value !== 'ready') return
  openedSource.value = mode.value === 'preview'
  mode.value = mode.value === 'preview' ? 'source' : 'preview'
}

const viewerOpen = ref(false)
const canExpand = computed(() => state.value === 'ready' && svg.value !== '')

function openViewer(): void {
  if (canExpand.value) viewerOpen.value = true
}

watch(
  () => [props.ready, props.source, dark.value, props.finished, settings.appearance.theme],
  draw,
  { immediate: true },
)
</script>

<template>
  <div class="mermaid-block" data-test="mermaid-block">
    <div class="mermaid-head-row">
      <button type="button" class="mermaid-head" :disabled="state !== 'ready'" @click="toggle">
        <span class="mermaid-title">图表 · mermaid</span>
        <span v-if="headLabel" class="mermaid-mode">{{ headLabel }}</span>
      </button>
      <button
        v-if="canExpand"
        type="button"
        class="mermaid-expand"
        data-test="mermaid-expand"
        title="放大查看"
        aria-label="放大查看图表"
        @click="openViewer"
      >
        ⤢
      </button>
    </div>

    <div
      v-if="state === 'waiting' || state === 'rendering' || state === 'incomplete'"
      class="mermaid-note"
      role="status"
      data-test="mermaid-waiting"
    >
      {{ note }}
    </div>

    <p v-if="state === 'failed'" class="mermaid-error" role="alert" data-test="mermaid-error">
      无法渲染: {{ reason }}
    </p>

    <div
      v-if="state === 'ready' && mode === 'preview'"
      class="mermaid-canvas"
      :class="{ 'is-dark': dark }"
      data-test="mermaid-canvas"
      v-html="svg"
    ></div>

    <pre
      v-if="state === 'failed' || state === 'incomplete' || mode === 'source'"
      class="mermaid-src"
      data-test="mermaid-source"
    >{{ source }}</pre>

    <DiagramViewer
      :svg="svg"
      :dark="dark"
      :open="viewerOpen"
      title="图表 · mermaid"
      @close="viewerOpen = false"
    />
  </div>
</template>

<style scoped>
.mermaid-block {
  display: block;
  clear: both;
  margin: calc(var(--space-unit) * 2) 0;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  overflow: hidden;
}

.mermaid-head-row {
  display: flex;
  align-items: stretch;
  border-bottom: 1px solid var(--color-border);
  background: var(--color-surface-muted);
}

.mermaid-head {
  display: flex;
  align-items: center;
  gap: var(--space-unit);
  flex: 1;
  min-width: 0;
  padding: var(--space-unit) calc(var(--space-unit) * 1.5);
  border: 0;
  background: transparent;
  color: var(--color-text);
  font: inherit;
  font-size: calc(var(--font-size-base) - 1px);
  text-align: left;
  cursor: pointer;
}

.mermaid-head:disabled {
  cursor: default;
}

.mermaid-expand {
  flex-shrink: 0;
  padding: 0 calc(var(--space-unit) * 1.5);
  border: 0;
  border-left: 1px solid var(--color-border);
  background: transparent;
  color: var(--color-primary);
  font: inherit;
  font-size: 1rem;
  line-height: 1;
  cursor: pointer;
}

.mermaid-expand:hover {
  background: var(--color-border);
}

.mermaid-title {
  color: var(--color-text);
}

.mermaid-mode {
  margin-left: auto;
  color: var(--color-primary);
}

.mermaid-note {
  padding: calc(var(--space-unit) * 2);
  color: var(--color-text-muted);
}

.mermaid-error {
  margin: 0;
  padding: var(--space-unit) calc(var(--space-unit) * 1.5);
  border-bottom: 1px solid var(--color-danger-border);
  color: var(--color-danger);
  background: var(--color-surface-muted);
}

.mermaid-canvas {
  padding: calc(var(--space-unit) * 2);
  background: var(--color-surface);
  color: var(--color-text);
  display: flex;
  justify-content: center;
  align-items: center;
  overflow-x: auto;
}

.mermaid-canvas.is-dark {
  background: #1e1e2e;
}

.mermaid-canvas :deep(svg) {
  display: block;
  max-width: 100%;
  height: auto;
}

.mermaid-canvas :deep(text) {
  font-family: var(--font-family) !important;
}

/* Ensure node labels, edge texts, and title texts are visible and use platform text color */
.mermaid-canvas :deep(.node .label text),
.mermaid-canvas :deep(.edgeLabel text),
.mermaid-canvas :deep(.cluster-label text),
.mermaid-canvas :deep(.nodeLabel),
.mermaid-canvas :deep(text.actor),
.mermaid-canvas :deep(text.messageText) {
  fill: var(--color-text) !important;
}

.mermaid-src {
  margin: 0;
  padding: calc(var(--space-unit) * 2);
  background: var(--color-surface-muted);
  color: var(--color-text);
  font-family: var(--font-mono, monospace);
  font-size: calc(var(--font-size-base) - 1px);
  line-height: 1.55;
  overflow: auto;
  white-space: pre;
  border-top: 1px solid var(--color-border);
}
</style>

<script setup lang="ts">
// A PlantUML source file opened in the workspace preview. It shows the drawing
// by default and the source one toggle away, like the diagram block in a chat
// reply, and it shares that block's renderer and enlarged viewer. A file that
// is only the diagram body gets its @startuml/@enduml markers, because the
// engine needs them; the source view always shows the file as it is.
import { computed, ref, watch } from 'vue'
import { renderDiagram } from '../../plantuml/renderer'
import PlantumlViewer from '../../shared/components/PlantumlViewer.vue'
import { useSettingsStore } from '../../stores/settings'
import { isDarkTheme } from '../../theme/presets'
import ArtifactToolbar from './ArtifactToolbar.vue'
import CodeView from './CodeView.vue'

const props = defineProps<{
  filePath: string
  content: string
  fileSize?: number
  lines?: number
}>()

const settings = useSettingsStore()
const dark = computed(() => isDarkTheme(settings.appearance.theme))

const mode = ref<'preview' | 'source'>('preview')
const state = ref<'rendering' | 'ready' | 'failed'>('rendering')
const svg = ref('')
const reason = ref('')
const viewerOpen = ref(false)
let request = 0

const drawingSource = computed(() =>
  /@start\w*/i.test(props.content) ? props.content : `@startuml\n${props.content}\n@enduml`,
)

async function draw(): Promise<void> {
  const token = ++request
  if (props.content.trim() === '') {
    svg.value = ''
    reason.value = '文件为空'
    state.value = 'failed'
    mode.value = 'source'
    return
  }
  state.value = 'rendering'
  const result = await renderDiagram({
    source: drawingSource.value,
    dark: dark.value,
    mode: settings.appearance.plantumlMode ?? 'local',
    serverUrl: settings.appearance.plantumlServerUrl ?? '',
  })
  if (token !== request) return // a newer file or palette superseded this drawing

  if (result.ok) {
    svg.value = result.svg
    reason.value = ''
    state.value = 'ready'
    mode.value = 'preview'
  } else {
    svg.value = ''
    reason.value = result.reason
    state.value = 'failed'
    mode.value = 'source'
  }
}

watch(
  () => [props.content, props.filePath, dark.value, settings.appearance.plantumlMode, settings.appearance.plantumlServerUrl],
  () => {
    void draw()
  },
  { immediate: true },
)
</script>

<template>
  <div class="plantuml-artifact-view" data-test="plantuml-artifact-view">
    <ArtifactToolbar
      v-model:mode="mode"
      :size="props.fileSize"
      :lines="props.lines"
      :file-path="props.filePath"
      :can-switch-mode="state === 'ready'"
    />

    <p v-if="state === 'failed'" class="artifact-error" role="alert" data-test="plantuml-artifact-error">
      无法渲染:{{ reason }}
    </p>

    <div class="view-content">
      <div v-if="state === 'rendering'" class="state-loading" role="status" data-test="plantuml-artifact-waiting">
        正在生成图…
      </div>

      <div
        v-else-if="state === 'ready' && mode === 'preview'"
        class="diagram-stage"
        :class="{ 'is-dark': dark }"
        data-test="plantuml-artifact-canvas"
      >
        <button
          type="button"
          class="expand-btn"
          data-test="plantuml-artifact-expand"
          title="放大查看"
          aria-label="放大查看图表"
          @click="viewerOpen = true"
        >
          ⤢
        </button>
        <div class="diagram-svg" v-html="svg"></div>
      </div>

      <CodeView
        v-if="mode === 'source'"
        :content="props.content"
        :file-path="props.filePath"
      />
    </div>

    <PlantumlViewer
      v-if="state === 'ready'"
      :svg="svg"
      :dark="dark"
      :open="viewerOpen"
      :title="props.filePath"
      @close="viewerOpen = false"
    />
  </div>
</template>

<style scoped>
.plantuml-artifact-view {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--color-surface, #ffffff);
  overflow: hidden;
}

.artifact-error {
  margin: 0;
  padding: 8px 12px;
  border-bottom: 1px solid var(--color-danger-border, #fca5a5);
  background: var(--color-surface-muted, #f8fafc);
  color: var(--color-danger, #b91c1c);
  font-size: 0.8rem;
}

.view-content {
  flex: 1;
  min-height: 0;
  position: relative;
  overflow: auto;
}

.state-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  font-size: 0.82rem;
  color: var(--color-text-muted);
}

.diagram-stage {
  position: relative;
  min-height: 100%;
  padding: 24px;
  background: #ffffff;
}

.diagram-stage.is-dark {
  background: #1e1e2e;
}

.diagram-svg :deep(svg) {
  display: block;
  max-width: 100%;
  height: auto;
  margin: 0 auto;
}

.expand-btn {
  position: absolute;
  top: 8px;
  right: 8px;
  z-index: 1;
  padding: 0 8px;
  border: 1px solid var(--color-border, #e3e6eb);
  border-radius: 4px;
  background: var(--color-surface, #ffffff);
  color: var(--color-primary, #2563eb);
  font-size: 1rem;
  line-height: 1.6;
  cursor: pointer;
}
</style>

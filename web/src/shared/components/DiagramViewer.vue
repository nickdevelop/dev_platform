<script setup lang="ts">
// Generic SVG diagram viewer modal with zoom, pan, and fullscreen support.
import PlantumlViewer from './PlantumlViewer.vue'

const props = withDefaults(
  defineProps<{
    svg: string
    dark: boolean
    open: boolean
    title?: string
  }>(),
  { title: '图表查看' },
)

const emit = defineEmits<{
  close: []
}>()
</script>

<template>
  <PlantumlViewer
    :svg="props.svg"
    :dark="props.dark"
    :open="props.open"
    :title="props.title"
    @close="emit('close')"
  />
</template>

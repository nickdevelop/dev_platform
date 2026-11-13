<script setup lang="ts">
import ServerDirectoryPicker from '../../shared/components/ServerDirectoryPicker.vue'

const props = defineProps<{
  initialPath?: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'select', path: string): void
}>()
</script>

<template>
  <ServerDirectoryPicker
    :initial-path="props.initialPath"
    @close="emit('close')"
    @select="(path: string) => emit('select', path)"
  />
</template>

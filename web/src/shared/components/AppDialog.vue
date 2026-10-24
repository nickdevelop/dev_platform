<script setup lang="ts">
import UiDialog from '../ui/compound/UiDialog.vue'

defineProps<{
  title: string
}>()

const emit = defineEmits<{ close: [] }>()
</script>

<template>
  <UiDialog :title="title" @close="emit('close')">
    <slot />
  </UiDialog>
</template>

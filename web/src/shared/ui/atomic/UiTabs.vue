<script setup lang="ts">
import { provide, toRef } from 'vue'

const props = withDefaults(
  defineProps<{
    modelValue: string | number
    variant?: 'line' | 'pill'
    size?: 'sm' | 'md'
  }>(),
  {
    variant: 'line',
    size: 'md',
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string | number]
  change: [value: string | number]
}>()

function selectTab(value: string | number) {
  emit('update:modelValue', value)
  emit('change', value)
}

provide('uiTabsContext', {
  activeValue: toRef(props, 'modelValue'),
  variant: toRef(props, 'variant'),
  size: toRef(props, 'size'),
  selectTab,
})
</script>

<template>
  <div
    class="flex items-center gap-1 border-b border-slate-200 dark:border-slate-800"
    :class="[
      variant === 'pill' ? 'border-b-0 bg-slate-100 dark:bg-slate-800/60 p-1 rounded' : '',
    ]"
    role="tablist"
  >
    <slot />
  </div>
</template>

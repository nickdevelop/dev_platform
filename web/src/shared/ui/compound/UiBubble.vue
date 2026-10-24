<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    align?: 'left' | 'right' | 'center'
    variant?: 'default' | 'primary' | 'muted'
    bordered?: boolean
  }>(),
  {
    align: 'left',
    variant: 'default',
    bordered: true,
  },
)

const variantClasses = computed(() => {
  switch (props.variant) {
    case 'primary':
      return 'bg-blue-50/70 dark:bg-blue-950/20 text-slate-800 dark:text-slate-100 border-blue-200 dark:border-blue-900/60'
    case 'muted':
      return 'bg-slate-50 dark:bg-slate-800/40 text-slate-600 dark:text-slate-300 border-slate-200 dark:border-slate-800'
    case 'default':
    default:
      return 'bg-white dark:bg-slate-900 text-slate-800 dark:text-slate-100 border-slate-200 dark:border-slate-800'
  }
})

const alignClasses = computed(() => {
  switch (props.align) {
    case 'right':
      return 'ml-auto text-left'
    case 'center':
      return 'mx-auto text-center'
    case 'left':
    default:
      return 'mr-auto text-left'
  }
})
</script>

<template>
  <div
    class="flex flex-col gap-1.5 w-full my-2.5"
    :class="[align === 'right' ? 'items-end' : align === 'center' ? 'items-center' : 'items-start']"
  >
    <div v-if="$slots.header" class="flex items-center gap-2 text-xs text-slate-400 dark:text-slate-500 px-1">
      <slot name="header" />
    </div>

    <div
      class="rounded text-sm leading-relaxed transition-colors duration-150 max-w-full"
      :class="[
        variantClasses,
        bordered ? 'border' : '',
        alignClasses,
      ]"
    >
      <div v-if="$slots.default" class="p-3.5">
        <slot />
      </div>

      <div v-if="$slots.expansion" class="border-t border-slate-100 dark:border-slate-800 bg-slate-50/50 dark:bg-slate-950/30 p-2.5 rounded-b">
        <slot name="expansion" />
      </div>
    </div>

    <div v-if="$slots.actions" class="flex items-center gap-2 text-xs text-slate-400 dark:text-slate-500 px-1">
      <slot name="actions" />
    </div>
  </div>
</template>

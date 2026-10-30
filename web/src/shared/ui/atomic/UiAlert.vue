<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    variant?: 'info' | 'success' | 'warning' | 'error'
    title?: string
    closable?: boolean
  }>(),
  {
    variant: 'info',
    title: '',
    closable: false,
  },
)

const emit = defineEmits<{
  close: []
}>()

const variantClasses = computed(() => {
  switch (props.variant) {
    case 'success':
      return 'bg-emerald-50 dark:bg-emerald-950/30 border-emerald-200 dark:border-emerald-800 text-emerald-800 dark:text-emerald-200'
    case 'warning':
      return 'bg-amber-50 dark:bg-amber-950/30 border-amber-200 dark:border-amber-800 text-amber-800 dark:text-amber-200'
    case 'error':
      return 'bg-red-50 dark:bg-red-950/30 border-red-200 dark:border-red-800 text-red-800 dark:text-red-200'
    case 'info':
    default:
      return 'bg-sky-50 dark:bg-sky-950/30 border-sky-200 dark:border-sky-800 text-sky-800 dark:text-sky-200'
  }
})

const iconColorClasses = computed(() => {
  switch (props.variant) {
    case 'success':
      return 'text-emerald-600 dark:text-emerald-400'
    case 'warning':
      return 'text-amber-600 dark:text-amber-400'
    case 'error':
      return 'text-red-600 dark:text-red-400'
    case 'info':
    default:
      return 'text-sky-600 dark:text-sky-400'
  }
})
</script>

<template>
  <div
    class="flex items-start gap-3 p-3 text-sm border rounded"
    :class="variantClasses"
    :role="variant === 'error' ? 'alert' : 'status'"
  >
    <div class="shrink-0 mt-0.5" :class="iconColorClasses">
      <svg v-if="variant === 'success'" class="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
        <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
      </svg>
      <svg v-else-if="variant === 'warning'" class="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
        <path fill-rule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clip-rule="evenodd" />
      </svg>
      <svg v-else-if="variant === 'error'" class="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
        <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd" />
      </svg>
      <svg v-else class="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
        <path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clip-rule="evenodd" />
      </svg>
    </div>

    <div class="flex-1 min-w-0">
      <h5 v-if="title" class="font-medium text-sm mb-0.5">{{ title }}</h5>
      <div class="text-xs leading-relaxed opacity-90 break-words">
        <slot />
      </div>
      <div v-if="$slots.actions" class="mt-2 flex items-center gap-2">
        <slot name="actions" />
      </div>
    </div>

    <button
      v-if="closable"
      type="button"
      class="shrink-0 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 inline-flex items-center justify-center"
      aria-label="关闭提示"
      @click="emit('close')"
    >
      <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8">
        <line x1="3" y1="3" x2="13" y2="13" />
        <line x1="13" y1="3" x2="3" y2="13" />
      </svg>
    </button>
  </div>
</template>

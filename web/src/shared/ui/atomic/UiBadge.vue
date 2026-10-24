<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    variant?: 'default' | 'primary' | 'success' | 'warning' | 'danger' | 'info'
    size?: 'sm' | 'md'
    dot?: boolean
    closable?: boolean
  }>(),
  {
    variant: 'default',
    size: 'md',
    dot: false,
    closable: false,
  },
)

const emit = defineEmits<{
  close: [event: MouseEvent]
}>()

const variantClasses = computed(() => {
  switch (props.variant) {
    case 'primary':
      return 'bg-blue-50 dark:bg-blue-950/40 text-blue-700 dark:text-blue-300 border-blue-200 dark:border-blue-800'
    case 'success':
      return 'bg-emerald-50 dark:bg-emerald-950/40 text-emerald-700 dark:text-emerald-300 border-emerald-200 dark:border-emerald-800'
    case 'warning':
      return 'bg-amber-50 dark:bg-amber-950/40 text-amber-700 dark:text-amber-300 border-amber-200 dark:border-amber-800'
    case 'danger':
      return 'bg-red-50 dark:bg-red-950/40 text-red-700 dark:text-red-300 border-red-200 dark:border-red-800'
    case 'info':
      return 'bg-sky-50 dark:bg-sky-950/40 text-sky-700 dark:text-sky-300 border-sky-200 dark:border-sky-800'
    case 'default':
    default:
      return 'bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 border-slate-200 dark:border-slate-700'
  }
})

const dotColorClasses = computed(() => {
  switch (props.variant) {
    case 'primary':
      return 'bg-blue-500'
    case 'success':
      return 'bg-emerald-500'
    case 'warning':
      return 'bg-amber-500'
    case 'danger':
      return 'bg-red-500'
    case 'info':
      return 'bg-sky-500'
    case 'default':
    default:
      return 'bg-slate-400'
  }
})
</script>

<template>
  <span
    class="inline-flex items-center font-medium border rounded select-none shrink-0"
    :class="[
      variantClasses,
      size === 'sm' ? 'px-1.5 py-0.5 text-[11px] gap-1' : 'px-2 py-0.5 text-xs gap-1.5',
    ]"
  >
    <span
      v-if="dot"
      data-test="badge-dot"
      class="w-1.5 h-1.5 rounded-full inline-block shrink-0"
      :class="dotColorClasses"
    />
    <slot />
    <button
      v-if="closable"
      type="button"
      class="hover:opacity-75 focus:outline-none ml-0.5"
      aria-label="移除标签"
      @click.stop="emit('close', $event)"
    >
      ✕
    </button>
  </span>
</template>

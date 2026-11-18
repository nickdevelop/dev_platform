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
      return 'bg-platform-primary-soft text-platform-primary border-platform-primary/25'
    case 'success':
      return 'bg-[var(--color-success-soft,#ecfdf5)] text-[var(--color-success,#15803d)] border-[var(--color-success-border,#86efac)]'
    case 'warning':
      return 'bg-[var(--color-warning-soft,#fffbeb)] text-[var(--color-warning,#92400e)] border-[var(--color-warning-border,#fcd34d)]'
    case 'danger':
      return 'bg-[var(--color-danger-soft,#fee2e2)] text-[var(--color-danger,#b91c1c)] border-[var(--color-danger-border,#fca5a5)]'
    case 'info':
      return 'bg-[var(--color-info-soft,#eff6ff)] text-[var(--color-info,#0369a1)] border-[var(--color-info-border,#93c5fd)]'
    case 'default':
    default:
      return 'bg-platform-surface-muted text-platform-text-muted border-platform-border'
  }
})

const dotColorClasses = computed(() => {
  switch (props.variant) {
    case 'primary':
      return 'bg-[var(--color-primary,#2563eb)]'
    case 'success':
      return 'bg-[var(--color-success,#15803d)]'
    case 'warning':
      return 'bg-[var(--color-warning,#92400e)]'
    case 'danger':
      return 'bg-[var(--color-danger,#b91c1c)]'
    case 'info':
      return 'bg-[var(--color-info,#0369a1)]'
    case 'default':
    default:
      return 'bg-platform-text-muted'
  }
})
</script>

<template>
  <span
    class="inline-flex items-center font-medium border rounded select-none shrink-0"
    :class="[
      variantClasses,
      size === 'sm' ? 'px-1.5 py-0.5 text-2xs gap-1' : 'px-2 py-0.5 text-xs gap-1.5',
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
      class="hover:opacity-75 focus:outline-none ml-0.5 inline-flex items-center justify-center"
      aria-label="移除标签"
      @click.stop="emit('close', $event)"
    >
      <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="1.8">
        <line x1="3" y1="3" x2="13" y2="13" />
        <line x1="13" y1="3" x2="3" y2="13" />
      </svg>
    </button>
  </span>
</template>

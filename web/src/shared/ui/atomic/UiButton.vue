<script setup lang="ts">
import { computed } from 'vue'
import UiSpinner from './UiSpinner.vue'

const props = withDefaults(
  defineProps<{
    variant?: 'primary' | 'secondary' | 'danger' | 'ghost' | 'link'
    size?: 'sm' | 'md' | 'lg'
    type?: 'button' | 'submit' | 'reset'
    disabled?: boolean
    loading?: boolean
    block?: boolean
  }>(),
  {
    variant: 'secondary',
    size: 'md',
    type: 'button',
    disabled: false,
    loading: false,
    block: false,
  },
)

const emit = defineEmits<{
  click: [event: MouseEvent]
}>()

const variantClasses = computed(() => {
  switch (props.variant) {
    case 'primary':
      return 'bg-platform-primary hover:bg-platform-primary-hover active:opacity-90 text-white border-transparent focus-visible:ring-platform-primary'
    case 'danger':
      return 'bg-platform-danger hover:opacity-90 active:opacity-80 text-white border-transparent focus-visible:ring-platform-danger'
    case 'ghost':
      return 'bg-transparent hover:bg-platform-surface-muted active:opacity-80 text-platform-text border-transparent focus-visible:ring-platform-primary'
    case 'link':
      return 'bg-transparent text-platform-primary hover:underline border-transparent p-0 h-auto font-normal focus-visible:ring-platform-primary'
    case 'secondary':
    default:
      return 'bg-platform-surface hover:bg-platform-surface-muted active:opacity-90 text-platform-text border-platform-border hover:border-platform-border-strong focus-visible:ring-platform-primary'
  }
})

const sizeClasses = computed(() => {
  if (props.variant === 'link') return ''
  switch (props.size) {
    case 'sm':
      return 'h-7 px-2 text-xs gap-1'
    case 'lg':
      return 'h-10 px-4 text-base gap-2'
    case 'md':
    default:
      return 'h-[var(--control-height,34px)] px-3 text-sm gap-1.5'
  }
})

function handleClick(e: MouseEvent) {
  if (props.disabled || props.loading) {
    e.preventDefault()
    e.stopPropagation()
    return
  }
  emit('click', e)
}
</script>

<template>
  <button
    :type="type"
    :disabled="disabled || loading"
    class="inline-flex items-center justify-center font-medium border rounded-[var(--radius,6px)] transition-colors duration-150 outline-none select-none focus-visible:ring-2 focus-visible:ring-offset-1 disabled:opacity-50 disabled:cursor-not-allowed disabled:pointer-events-none"
    :class="[
      variantClasses,
      sizeClasses,
      block ? 'w-full' : '',
    ]"
    @click="handleClick"
  >
    <UiSpinner v-if="loading" :size="size === 'lg' ? 'md' : 'sm'" />
    <slot name="prefix" />
    <slot />
    <slot name="suffix" />
  </button>
</template>

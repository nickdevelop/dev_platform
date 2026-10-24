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
      return 'bg-blue-600 hover:bg-blue-700 active:bg-blue-800 text-white border-transparent focus-visible:ring-blue-500'
    case 'danger':
      return 'bg-red-600 hover:bg-red-700 active:bg-red-800 text-white border-transparent focus-visible:ring-red-500'
    case 'ghost':
      return 'bg-transparent hover:bg-slate-100 dark:hover:bg-slate-800 active:bg-slate-200 dark:active:bg-slate-700 text-slate-700 dark:text-slate-300 border-transparent focus-visible:ring-slate-400'
    case 'link':
      return 'bg-transparent text-blue-600 hover:text-blue-700 dark:text-blue-400 hover:underline border-transparent p-0 h-auto font-normal focus-visible:ring-blue-400'
    case 'secondary':
    default:
      return 'bg-white dark:bg-slate-800 hover:bg-slate-50 dark:hover:bg-slate-700 active:bg-slate-100 dark:active:bg-slate-600 text-slate-700 dark:text-slate-200 border-slate-300 dark:border-slate-700 focus-visible:ring-blue-500'
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
      return 'h-[34px] px-3 text-sm gap-1.5'
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
    class="inline-flex items-center justify-center font-medium border rounded transition-colors duration-150 outline-none select-none focus-visible:ring-2 focus-visible:ring-offset-1 dark:focus-visible:ring-offset-slate-900 disabled:opacity-50 disabled:cursor-not-allowed disabled:pointer-events-none"
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

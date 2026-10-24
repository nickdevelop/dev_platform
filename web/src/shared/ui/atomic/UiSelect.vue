<script setup lang="ts">
import { computed } from 'vue'

defineOptions({
  inheritAttrs: false,
})

export interface SelectOption {
  label: string
  value: string | number
  disabled?: boolean
}

const props = withDefaults(
  defineProps<{
    modelValue?: string | number | null
    options?: SelectOption[]
    size?: 'sm' | 'md' | 'lg'
    disabled?: boolean
    error?: boolean | string
    block?: boolean
  }>(),
  {
    modelValue: '',
    options: undefined,
    size: 'md',
    disabled: false,
    error: false,
    block: false,
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string | number]
  change: [event: Event]
}>()

const sizeClasses = computed(() => {
  switch (props.size) {
    case 'sm':
      return 'h-7 text-xs pl-2 pr-7'
    case 'lg':
      return 'h-10 text-base pl-3.5 pr-9'
    case 'md':
    default:
      return 'h-[34px] text-sm pl-2.5 pr-8'
  }
})

function onChange(event: Event) {
  const target = event.target as HTMLSelectElement
  emit('update:modelValue', target.value)
  emit('change', event)
}
</script>

<template>
  <div
    class="relative inline-flex items-center"
    :class="[block ? 'w-full flex' : '']"
  >
    <select
      :value="modelValue ?? ''"
      :disabled="disabled"
      v-bind="$attrs"
      class="appearance-none w-full rounded bg-white dark:bg-slate-900 border text-slate-800 dark:text-slate-100 outline-none transition-colors duration-150 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed select-none"
      :class="[
        sizeClasses,
        error
          ? 'border-red-500 focus:border-red-500 focus:ring-2 focus:ring-red-500/20'
          : 'border-slate-300 dark:border-slate-700 hover:border-slate-400 dark:hover:border-slate-600 focus:border-blue-500 dark:focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20',
      ]"
      @change="onChange"
    >
      <template v-if="options">
        <option
          v-for="opt in options"
          :key="opt.value"
          :value="opt.value"
          :disabled="opt.disabled"
        >
          {{ opt.label }}
        </option>
      </template>
      <slot v-else />
    </select>

    <div class="pointer-events-none absolute right-2.5 flex items-center text-slate-400 dark:text-slate-500">
      <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M6 9l6 6 6-6" />
      </svg>
    </div>
  </div>
</template>

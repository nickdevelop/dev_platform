<script setup lang="ts">
import { computed } from 'vue'

defineOptions({
  inheritAttrs: false,
})

const props = withDefaults(
  defineProps<{
    modelValue?: string | number
    type?: string
    size?: 'sm' | 'md' | 'lg'
    placeholder?: string
    disabled?: boolean
    readonly?: boolean
    error?: boolean | string
    clearable?: boolean
    block?: boolean
  }>(),
  {
    modelValue: '',
    type: 'text',
    size: 'md',
    placeholder: '',
    disabled: false,
    readonly: false,
    error: false,
    clearable: false,
    block: false,
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
  clear: []
  blur: [event: FocusEvent]
  focus: [event: FocusEvent]
  change: [event: Event]
}>()

const sizeClasses = computed(() => {
  switch (props.size) {
    case 'sm':
      return 'h-7 text-xs px-2'
    case 'lg':
      return 'h-10 text-base px-3.5'
    case 'md':
    default:
      return 'h-[var(--control-height,34px)] text-sm px-2.5'
  }
})

function onInput(event: Event) {
  emit('update:modelValue', (event.target as HTMLInputElement).value)
}

function handleClear() {
  emit('update:modelValue', '')
  emit('clear')
}
</script>

<template>
  <div
    class="relative inline-flex items-center text-platform-text"
    :class="[block ? 'w-full flex' : '']"
  >
    <div
      v-if="$slots.prefix"
      class="absolute left-2.5 flex items-center pointer-events-none text-platform-text-muted"
    >
      <slot name="prefix" />
    </div>

    <input
      :type="type"
      :value="modelValue"
      :placeholder="placeholder"
      :disabled="disabled"
      :readonly="readonly"
      v-bind="$attrs"
      class="w-full rounded-[var(--radius,6px)] bg-platform-surface border text-platform-text placeholder-platform-text-muted outline-none transition-colors duration-150 disabled:opacity-50 disabled:cursor-not-allowed"
      :class="[
        sizeClasses,
        $slots.prefix ? 'pl-8' : '',
        $slots.suffix || clearable ? 'pr-8' : '',
        error
          ? 'border-platform-danger focus:border-platform-danger focus:ring-2 focus:ring-platform-danger/20'
          : 'border-platform-border hover:border-platform-border-strong focus:border-platform-primary focus:ring-2 focus:ring-platform-primary/20',
      ]"
      @input="onInput"
      @change="$emit('change', $event)"
      @focus="$emit('focus', $event)"
      @blur="$emit('blur', $event)"
    />

    <button
      v-if="clearable && !disabled && !readonly && modelValue !== ''"
      type="button"
      class="absolute right-2 flex items-center justify-center w-4 h-4 rounded-full text-platform-text-muted hover:text-platform-text"
      title="清空"
      aria-label="清空内容"
      @click="handleClear"
    >
      <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.8">
        <line x1="3" y1="3" x2="13" y2="13" />
        <line x1="13" y1="3" x2="3" y2="13" />
      </svg>
    </button>

    <div
      v-else-if="$slots.suffix"
      class="absolute right-2.5 flex items-center pointer-events-none text-slate-400 dark:text-slate-500"
    >
      <slot name="suffix" />
    </div>
  </div>
</template>

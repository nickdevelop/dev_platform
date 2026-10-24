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
      return 'h-[34px] text-sm px-2.5'
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
    class="relative inline-flex items-center text-slate-800 dark:text-slate-100"
    :class="[block ? 'w-full flex' : '']"
  >
    <div
      v-if="$slots.prefix"
      class="absolute left-2.5 flex items-center pointer-events-none text-slate-400 dark:text-slate-500"
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
      class="w-full rounded bg-white dark:bg-slate-900 border text-slate-900 dark:text-slate-100 placeholder-slate-400 dark:placeholder-slate-500 outline-none transition-colors duration-150 disabled:opacity-50 disabled:cursor-not-allowed"
      :class="[
        sizeClasses,
        $slots.prefix ? 'pl-8' : '',
        $slots.suffix || clearable ? 'pr-8' : '',
        error
          ? 'border-red-500 focus:border-red-500 focus:ring-2 focus:ring-red-500/20'
          : 'border-slate-300 dark:border-slate-700 hover:border-slate-400 dark:hover:border-slate-600 focus:border-blue-500 dark:focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20',
      ]"
      @input="onInput"
      @change="$emit('change', $event)"
      @focus="$emit('focus', $event)"
      @blur="$emit('blur', $event)"
    />

    <button
      v-if="clearable && !disabled && !readonly && modelValue !== ''"
      type="button"
      class="absolute right-2 flex items-center justify-center w-4 h-4 rounded-full text-slate-400 hover:text-slate-600 dark:hover:text-slate-200"
      title="清空"
      aria-label="清空内容"
      @click="handleClear"
    >
      ✕
    </button>

    <div
      v-else-if="$slots.suffix"
      class="absolute right-2.5 flex items-center pointer-events-none text-slate-400 dark:text-slate-500"
    >
      <slot name="suffix" />
    </div>
  </div>
</template>

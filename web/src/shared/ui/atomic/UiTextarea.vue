<script setup lang="ts">
import { nextTick, onMounted, ref, watch } from 'vue'

defineOptions({
  inheritAttrs: false,
})

const props = withDefaults(
  defineProps<{
    modelValue?: string
    placeholder?: string
    disabled?: boolean
    readonly?: boolean
    rows?: number
    autoResize?: boolean
    maxHeight?: number
    error?: boolean | string
  }>(),
  {
    modelValue: '',
    placeholder: '',
    disabled: false,
    readonly: false,
    rows: 3,
    autoResize: false,
    maxHeight: 400,
    error: false,
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
  blur: [event: FocusEvent]
  focus: [event: FocusEvent]
  keydown: [event: KeyboardEvent]
}>()

const textareaRef = ref<HTMLTextAreaElement | null>(null)

function resize() {
  if (!props.autoResize || !textareaRef.value) return
  const el = textareaRef.value
  el.style.height = 'auto'
  const newHeight = Math.min(el.scrollHeight, props.maxHeight)
  el.style.height = `${newHeight}px`
}

function onInput(event: Event) {
  const val = (event.target as HTMLTextAreaElement).value
  emit('update:modelValue', val)
  void nextTick(resize)
}

watch(() => props.modelValue, () => {
  void nextTick(resize)
})

onMounted(() => {
  void nextTick(resize)
})

defineExpose({
  textareaRef,
  resize,
})
</script>

<template>
  <div class="w-full relative">
    <textarea
      ref="textareaRef"
      :value="modelValue"
      :rows="rows"
      :placeholder="placeholder"
      :disabled="disabled"
      :readonly="readonly"
      v-bind="$attrs"
      class="w-full rounded bg-white dark:bg-slate-900 border text-slate-900 dark:text-slate-100 placeholder-slate-400 dark:placeholder-slate-500 p-2 text-sm outline-none transition-colors duration-150 disabled:opacity-50 disabled:cursor-not-allowed resize-y"
      :class="[
        autoResize ? 'resize-none' : '',
        error
          ? 'border-red-500 focus:border-red-500 focus:ring-2 focus:ring-red-500/20'
          : 'border-slate-300 dark:border-slate-700 hover:border-slate-400 dark:hover:border-slate-600 focus:border-blue-500 dark:focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20',
      ]"
      @input="onInput"
      @keydown="$emit('keydown', $event)"
      @focus="$emit('focus', $event)"
      @blur="$emit('blur', $event)"
    ></textarea>
  </div>
</template>

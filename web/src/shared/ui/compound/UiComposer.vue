<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'

const props = withDefaults(
  defineProps<{
    modelValue: string
    placeholder?: string
    disabled?: boolean
    readonly?: boolean
    compact?: boolean
    minHeight?: number
    maxHeight?: number
    autoResize?: boolean
    showResizer?: boolean
    inputId?: string
  }>(),
  {
    placeholder: '输入内容…',
    disabled: false,
    readonly: false,
    compact: false,
    minHeight: 36,
    maxHeight: 400,
    autoResize: true,
    showResizer: true,
    inputId: 'ui-composer-input',
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
  submit: []
  keydown: [event: KeyboardEvent]
  paste: [event: ClipboardEvent]
  drop: [event: DragEvent]
}>()

const textareaRef = ref<HTMLTextAreaElement | null>(null)
const manualHeight = ref<number | null>(null)
let isResizing = false
let startY = 0
let startHeight = 0

function onResizerMouseDown(e: MouseEvent) {
  if (!textareaRef.value) return
  isResizing = true
  startY = e.clientY
  startHeight = textareaRef.value.getBoundingClientRect().height
  document.body.style.cursor = 'ns-resize'
  document.body.style.userSelect = 'none'

  const onMouseMove = (ev: MouseEvent) => {
    if (!isResizing) return
    const delta = startY - ev.clientY
    const newHeight = Math.max(props.minHeight, Math.min(props.maxHeight, startHeight + delta))
    manualHeight.value = newHeight
  }

  const onMouseUp = () => {
    isResizing = false
    document.body.style.cursor = ''
    document.body.style.userSelect = ''
    window.removeEventListener('mousemove', onMouseMove)
    window.removeEventListener('mouseup', onMouseUp)
  }

  window.addEventListener('mousemove', onMouseMove)
  window.addEventListener('mouseup', onMouseUp)
}

function resetComposerHeight() {
  manualHeight.value = null
  void nextTick(autoAdjustHeight)
}

function autoAdjustHeight() {
  if (manualHeight.value !== null || !props.autoResize || !textareaRef.value) return
  const el = textareaRef.value
  el.style.height = 'auto'
  const targetMax = props.compact ? 120 : props.maxHeight
  el.style.height = `${Math.min(el.scrollHeight, targetMax)}px`
}

function onInput(event: Event) {
  const val = (event.target as HTMLTextAreaElement).value
  emit('update:modelValue', val)
  void nextTick(autoAdjustHeight)
}

watch(() => props.modelValue, () => {
  void nextTick(autoAdjustHeight)
})

defineExpose({
  textareaRef,
  resetComposerHeight,
})
</script>

<template>
  <div
    class="relative rounded border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 transition-colors focus-within:border-blue-500 dark:focus-within:border-blue-500 focus-within:ring-2 focus-within:ring-blue-500/20"
    @dragover.prevent
    @drop="$emit('drop', $event)"
  >
    <div
      v-if="showResizer && !compact"
      class="h-2 w-full flex items-center justify-center cursor-ns-resize select-none group hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors rounded-t"
      title="拖拽调整输入框高度，双击重置"
      @mousedown="onResizerMouseDown"
      @dblclick="resetComposerHeight"
    >
      <span class="w-8 h-1 rounded-full bg-slate-300 dark:bg-slate-700 group-hover:bg-slate-400 dark:group-hover:bg-slate-500 transition-colors" />
    </div>

    <div v-if="$slots.banner" class="px-3 pt-2">
      <slot name="banner" />
    </div>

    <div class="px-3 pt-2">
      <label :for="inputId" class="sr-only">输入内容</label>
      <textarea
        :id="inputId"
        ref="textareaRef"
        :value="modelValue"
        :placeholder="placeholder"
        :disabled="disabled"
        :readonly="readonly"
        :rows="compact ? 2 : 3"
        :style="manualHeight ? { height: `${manualHeight}px`, maxHeight: 'none' } : undefined"
        class="w-full bg-transparent text-slate-800 dark:text-slate-100 placeholder-slate-400 dark:placeholder-slate-500 text-sm outline-none resize-none leading-relaxed"
        @input="onInput"
        @keydown="$emit('keydown', $event)"
        @paste="$emit('paste', $event)"
      />
    </div>

    <div v-if="$slots.attachments" class="px-3 py-1">
      <slot name="attachments" />
    </div>

    <div class="flex items-center justify-between gap-2 px-3 py-2 border-t border-slate-100 dark:border-slate-800/80">
      <div class="flex items-center gap-1.5 flex-wrap">
        <slot name="tools" />
      </div>
      <div class="flex items-center gap-2 shrink-0">
        <slot name="actions" />
      </div>
    </div>
  </div>
</template>

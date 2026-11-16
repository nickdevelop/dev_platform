<script setup lang="ts">
// A user turn's text shows its first FOLD_LINES visual lines and folds the rest
// behind a toggle. The fold is measured from the rendered height, so a wrapped
// line counts as a line and the fold follows the bubble width.
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = defineProps<{ text: string }>()

const FOLD_LINES = 10
const textEl = ref<HTMLElement | null>(null)
const expanded = ref(false)
const overflowing = ref(false)
const foldHeight = ref(0)
let observer: ResizeObserver | null = null

function measure() {
  const el = textEl.value
  if (!el) return
  const style = getComputedStyle(el)
  const lineHeight = parseFloat(style.lineHeight) || parseFloat(style.fontSize) * 1.5
  foldHeight.value = lineHeight * FOLD_LINES
  overflowing.value = el.scrollHeight > foldHeight.value + 1
}

watch(
  () => props.text,
  () => void nextTick(measure),
)

onMounted(() => {
  measure()
  if (typeof ResizeObserver !== 'undefined' && textEl.value) {
    observer = new ResizeObserver(measure)
    observer.observe(textEl.value)
  }
})

onBeforeUnmount(() => {
  observer?.disconnect()
})
</script>

<template>
  <div class="user-text">
    <div
      ref="textEl"
      class="message-text user-text-body"
      :class="{ 'is-folded': overflowing && !expanded }"
      :style="overflowing && !expanded ? { maxHeight: `${foldHeight}px` } : undefined"
      data-test="user-text-body"
    >{{ props.text }}</div>
    <button
      v-if="overflowing"
      type="button"
      class="user-text-toggle"
      data-test="user-text-toggle"
      :aria-expanded="expanded"
      @click="expanded = !expanded"
    >{{ expanded ? '收起' : '展开' }}</button>
  </div>
</template>

<style scoped>
.user-text-body {
  white-space: pre-wrap;
  word-break: break-word;
}

.user-text-body.is-folded {
  overflow: hidden;
}

.user-text-toggle {
  display: block;
  margin-top: 0.25rem;
  padding: 0;
  border: none;
  background: none;
  color: inherit;
  font-size: 0.75rem;
  opacity: 0.75;
  text-decoration: underline;
  text-underline-offset: 2px;
  cursor: pointer;
}

.user-text-toggle:hover {
  opacity: 1;
}
</style>

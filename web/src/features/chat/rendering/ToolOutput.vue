<script setup lang="ts">
// A plain-text tool output clipped to a few rows: the first `maxLines` lines
// render with a bottom fade and a centered ▼ that expands to the full text (and
// folds it back). Shell commands, edit fallbacks and every other textual result
// share this, so a long log never dominates the transcript.
//
// Rows are counted by newline, not by wrapped visual lines: a "line of data" is
// what a tool actually printed. A trailing newline does not count as a row.
import { computed, ref } from 'vue'
import ExpandIndicator from './ExpandIndicator.vue'

const props = withDefaults(
  defineProps<{ text: string; maxLines?: number; isError?: boolean }>(),
  { maxLines: 15 },
)

const collapsed = ref(true)

const lines = computed(() => {
  const all = props.text.split('\n')
  if (all.length > 1 && all[all.length - 1] === '') all.pop()
  return all
})

const truncated = computed(() => lines.value.length > props.maxLines)

const visibleText = computed(() =>
  truncated.value && collapsed.value
    ? lines.value.slice(0, props.maxLines).join('\n')
    : lines.value.join('\n'),
)

function toggle() {
  collapsed.value = !collapsed.value
}
</script>

<template>
  <div class="tool-output-wrap">
    <pre class="tool-output" :class="{ 'is-error': isError }" data-test="tool-output">{{ visibleText }}</pre>
    <ExpandIndicator v-if="truncated" :collapsed="collapsed" @toggle="toggle" />
  </div>
</template>

<style scoped>
.tool-output-wrap {
  position: relative;
}

.tool-output {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: var(--font-family);
  font-size: 0.8rem;
  color: var(--color-text);
}

.tool-output.is-error {
  color: var(--color-danger);
}
</style>

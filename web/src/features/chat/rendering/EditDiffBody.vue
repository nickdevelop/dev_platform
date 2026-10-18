<script setup lang="ts">
// Renders one unified patch as colored diff rows: green additions, red
// deletions, muted hunk headers, plain context. Parsing is line-based with
// no new dependencies; anything that is not a +/-/@@ line renders as
// context so a malformed patch degrades to readable text instead of failing.
//
// Only the first VISIBLE_LIMIT rows show by default, with a bottom fade and a
// centered ▼ that expands to the rest (and folds back). `maxLines` stays the
// hard safety cap: a patch longer than it is cut with an explicit note instead
// of freezing the browser on a huge diff.
import { computed, ref } from 'vue'
import ExpandIndicator from './ExpandIndicator.vue'

const props = defineProps<{ patch?: string; maxLines?: number }>()

type DiffRow = { kind: 'hunk' | 'addition' | 'deletion' | 'context'; text: string }

const LIMIT = 400
const VISIBLE_LIMIT = 15

const collapsed = ref(true)

const rows = computed<DiffRow[]>(() => {
  if (!props.patch) return []
  const cap = props.maxLines ?? LIMIT
  const out: DiffRow[] = []
  for (const raw of props.patch.split('\n')) {
    if (out.length >= cap) break
    if (raw.startsWith('@@')) out.push({ kind: 'hunk', text: raw })
    else if (raw.startsWith('+') && !raw.startsWith('+++')) out.push({ kind: 'addition', text: raw.slice(1) })
    else if (raw.startsWith('-') && !raw.startsWith('---')) out.push({ kind: 'deletion', text: raw.slice(1) })
    else if (raw.startsWith('---') || raw.startsWith('+++')) continue
    else out.push({ kind: 'context', text: raw.startsWith(' ') ? raw.slice(1) : raw })
  }
  return out
})

const foldable = computed(() => rows.value.length > VISIBLE_LIMIT)

const visibleRows = computed(() =>
  foldable.value && collapsed.value ? rows.value.slice(0, VISIBLE_LIMIT) : rows.value,
)

// The patch was cut at the hard cap, so even the expanded view omits rows.
const clipped = computed(() =>
  props.patch ? props.patch.split('\n').length > (props.maxLines ?? LIMIT) : false,
)

function toggle() {
  collapsed.value = !collapsed.value
}
</script>

<template>
  <div class="edit-diff-wrap">
    <div class="edit-diff" data-test="edit-diff">
      <div
        v-for="(row, index) in visibleRows"
        :key="index"
        class="diff-row"
        :class="`diff-${row.kind}`"
      >
        <span class="diff-sign" aria-hidden="true">{{
          row.kind === 'addition' ? '+' : row.kind === 'deletion' ? '-' : row.kind === 'hunk' ? '●' : ' '
        }}</span>
        <code class="diff-text">{{ row.text }}</code>
      </div>
      <p v-if="clipped && !collapsed" class="diff-truncated">… 后续行已省略</p>
    </div>
    <ExpandIndicator v-if="foldable" :collapsed="collapsed" @toggle="toggle" />
  </div>
</template>

<style scoped>
.edit-diff-wrap {
  position: relative;
}

.edit-diff {
  display: flex;
  flex-direction: column;
  gap: 1px;
  font-family: var(--font-family);
  font-size: 0.78rem;
  line-height: 1.55;
  overflow-x: auto;
}

.diff-row {
  display: flex;
  gap: 0.5rem;
  padding: 0.05rem 0.5rem;
  border-radius: 4px;
  min-width: 0;
}

.diff-sign {
  flex-shrink: 0;
  width: 1em;
  text-align: center;
  opacity: 0.7;
  user-select: none;
}

.diff-text {
  white-space: pre-wrap;
  word-break: break-word;
  overflow-wrap: anywhere;
  min-width: 0;
}

.diff-addition {
  background: color-mix(in srgb, var(--color-success, #16a34a) 12%, transparent);
  color: var(--color-text);
}

.diff-deletion {
  background: color-mix(in srgb, var(--color-danger, #dc2626) 10%, transparent);
  color: var(--color-text);
}

.diff-hunk {
  color: var(--color-text-muted);
  font-size: 0.72rem;
}

.diff-truncated {
  margin: 0.25rem 0 0;
  color: var(--color-text-muted);
  font-size: 0.75rem;
}
</style>

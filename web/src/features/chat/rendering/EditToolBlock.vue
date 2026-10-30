<script setup lang="ts">
// Desktop-style edit tool block: header carries the file path plus the
// +N/-N badge, the body carries the unified diff. Without a patch (old
// transcripts, pi display-only diffs) it degrades to the plain text output
// so nothing ever renders empty.
import { computed } from 'vue'
import EditDiffBadge from './EditDiffBadge.vue'
import EditDiffBody from './EditDiffBody.vue'
import ToolOutput from './ToolOutput.vue'

const props = defineProps<{
  filePath: string
  additions?: number
  deletions?: number
  patch?: string
  output?: string
  isError?: boolean
}>()

const hasCounts = computed(
  () => (props.additions ?? 0) > 0 || (props.deletions ?? 0) > 0,
)
</script>

<template>
  <div class="edit-tool" data-test="edit-tool">
    <div class="edit-head">
      <span class="tool-icon" aria-hidden="true">
        <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8">
          <path d="M11 2.5l2.5 2.5L5 13.5H2.5V11L11 2.5z" />
        </svg>
      </span>
      <span class="tool-name">edit</span>
      <span v-if="filePath" class="tool-arg" data-test="edit-path">{{ filePath }}</span>
      <EditDiffBadge :additions="additions" :deletions="deletions" />
    </div>
    <div class="edit-body">
      <EditDiffBody v-if="patch" :patch="patch" />
      <ToolOutput v-else-if="output" :text="output" :is-error="isError" data-test="edit-output" />
      <p v-else-if="hasCounts" class="edit-counts" data-test="edit-counts">
        <template v-if="(additions ?? 0) > 0">+{{ additions }} 行</template>
        <template v-if="(additions ?? 0) > 0 && (deletions ?? 0) > 0"> / </template>
        <template v-if="(deletions ?? 0) > 0">-{{ deletions }} 行</template>
      </p>
    </div>
  </div>
</template>

<style scoped>
.edit-head {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.85rem;
  min-width: 0;
}

.tool-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--color-text-muted);
}

.edit-head .tool-arg {
  flex: 1;
}

.edit-body {
  margin-top: 0.4rem;
  padding: 0.5rem 0.6rem;
  border-radius: 8px;
  background: var(--color-surface-muted);
}

.edit-counts {
  margin: 0;
  font-size: 0.8rem;
  color: var(--color-text-muted);
}
</style>

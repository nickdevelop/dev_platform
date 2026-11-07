<script setup lang="ts">
// FlowRoleBadge: Compact, neutral role pill with color accent indicator
import { computed } from 'vue'
import type { FlowRole } from '../../../api/tasks'

const props = defineProps<{
  role?: FlowRole | null
  name?: string
  color?: string
  backend?: string
  model?: string
  interactive?: boolean
}>()

const roleName = computed(() => props.role?.name || props.name || 'Agent')
const roleColor = computed(() => props.role?.color || props.color || '#4a8ad6')
const roleBackend = computed(() => props.role?.backend || props.backend || 'llm')
const roleModel = computed(() => props.role?.model || props.model || '')
</script>

<template>
  <span
    class="flow-role-badge inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-2xs font-medium border border-[var(--color-border)] bg-[var(--color-surface-muted)] text-[var(--color-text)] transition-colors select-none"
    :class="{ 'hover:border-[var(--color-primary)] cursor-pointer': interactive }"
  >
    <span
      class="w-1.5 h-1.5 rounded-full shrink-0"
      :style="{ backgroundColor: roleColor }"
      aria-hidden="true"
    />
    <span class="role-name font-semibold tracking-tight">{{ roleName }}</span>
    <span
      v-if="roleBackend"
      class="text-3xs text-[var(--color-text-muted)] border-l border-[var(--color-border)] pl-1.5 font-mono"
    >
      {{ roleBackend }}
    </span>
    <span
      v-if="roleModel"
      class="text-3xs text-[var(--color-text-muted)] truncate max-w-[90px] font-mono"
      :title="roleModel"
    >
      {{ roleModel }}
    </span>
  </span>
</template>

<style scoped>
.flow-role-badge {
  font-variant-numeric: tabular-nums;
}
</style>

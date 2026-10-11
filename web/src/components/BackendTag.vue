<script setup lang="ts">
// Agent backend tag shown on tasks, the chat header and the new-task form.
// Colors are fixed per backend so a task's agent is readable at a glance.
import { computed } from 'vue'
import type { BackendId } from '../api/agents'

const props = defineProps<{ backend: BackendId }>()

const label = computed(() => (props.backend === 'pi' ? 'pi' : 'opencode'))
</script>

<template>
  <span class="tag" :class="props.backend === 'pi' ? 'pi' : 'opencode'">{{ label }}</span>
</template>

<style scoped>
.tag {
  font-size: 0.786rem;
  padding: calc(var(--space-unit) * 0.167) calc(var(--space-unit) * 1.000);
  border-radius: 4px;
  white-space: nowrap;
}

.tag.opencode {
  background: var(--color-info-soft);
  color: var(--color-info);
}

.tag.pi {
  background: var(--color-warning-soft);
  color: var(--color-warning);
}
</style>

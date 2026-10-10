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
  font-size: 11px;
  padding: 1px 6px;
  border-radius: 4px;
  white-space: nowrap;
}

.tag.opencode {
  background: #e0f2fe;
  color: #0369a1;
}

.tag.pi {
  background: #fef3c7;
  color: #92400e;
}
</style>

<script setup lang="ts">
import ModelSelect from '../../../shared/components/ModelSelect.vue'
import type { BackendId } from '../../../api/agents'

const props = defineProps<{
  backend: BackendId
  modelSelection: boolean
  modelValue: string
}>()

const emit = defineEmits<{
  'update:modelValue': [id: string]
  picked: [id: string]
  status: [ok: boolean]
  failed: [failed: boolean]
}>()
</script>

<template>
  <ModelSelect
    :backend="props.backend"
    :model-selection="props.modelSelection"
    :model-value="props.modelValue"
    @update:model-value="(id: string) => emit('update:modelValue', id)"
    @picked="(id: string) => emit('picked', id)"
    @status="(ok: boolean) => emit('status', ok)"
    @failed="(failed: boolean) => emit('failed', failed)"
  />
</template>

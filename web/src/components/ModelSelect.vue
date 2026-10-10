<script setup lang="ts">
// Model picker for the composer. It loads the open instance's model list once
// per instance and reports its load status to the parent, which gates sending
// so a failed list can never fall back to a free-text model. Rendered only when
// the session declares modelSelection.
import { onMounted, ref, watch } from 'vue'
import { listInstanceModels, type ModelInfo } from '../api/agents'

const props = defineProps<{
  projectId: number
  instanceId: string
  modelSelection: boolean
  modelValue: string
}>()

const emit = defineEmits<{
  'update:modelValue': [id: string]
  status: [ok: boolean]
}>()

const models = ref<ModelInfo[]>([])
const loading = ref(false)
const failed = ref(false)

async function load() {
  if (!props.modelSelection || props.instanceId === '') return
  loading.value = true
  failed.value = false
  try {
    const list = await listInstanceModels(props.projectId, props.instanceId)
    models.value = list.models
    emit('status', true)
    // Default to the first model when the parent has not chosen one.
    const selected = props.modelValue || list.models[0]?.id || ''
    if (selected !== '' && selected !== props.modelValue) emit('update:modelValue', selected)
  } catch {
    models.value = []
    failed.value = true
    emit('status', false)
  } finally {
    loading.value = false
  }
}

onMounted(load)
watch(() => [props.projectId, props.instanceId, props.modelSelection], load)

// When the parent clears the selection (switching to another task on the same
// instance does not reload the list), default to the first model again so the
// picker is never left blank.
watch(
  () => props.modelValue,
  (value) => {
    if (props.modelSelection && value === '' && models.value.length > 0) {
      emit('update:modelValue', models.value[0].id)
    }
  },
)

function onSelect(event: Event) {
  emit('update:modelValue', (event.target as HTMLSelectElement).value)
}
</script>

<template>
  <div v-if="props.modelSelection" class="model-select">
    <p v-if="failed" class="model-error" role="alert">模型列表加载失败</p>
    <select
      v-else
      class="model-options"
      :value="props.modelValue"
      :disabled="loading"
      aria-label="模型"
      @change="onSelect"
    >
      <option v-for="model in models" :key="model.id" :value="model.id">
        {{ model.name || model.id }}
      </option>
    </select>
  </div>
</template>

<style scoped>
.model-select {
  display: flex;
  align-items: center;
}

.model-options {
  padding: 0.25rem 0.4rem;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  font: inherit;
  background: #fff;
}

.model-error {
  margin: 0;
  color: #b91c1c;
  font-size: 0.8rem;
}
</style>

<script setup lang="ts">
// Model picker for the composer. It shows the backend's last known model list
// (from the in-memory cache) the moment a task opens, then refreshes it from the
// open instance in the background. A failed refresh with a cache present is
// silent: the user already has models and sending stays enabled. With no cache,
// a failure is reported and sending is gated so a free-text model is never used.
import { onMounted, ref, watch } from 'vue'
import { listInstanceModels, type BackendId, type ModelInfo } from '../api/agents'
import { cachedModels, rememberModels } from '../modelCache'

const props = defineProps<{
  projectId: number
  instanceId: string
  backend: BackendId
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
// A token per request: a response that arrives after a newer one was issued is
// discarded, so switching tasks can never show the previous task's models.
let requestToken = 0

// applyModels shows a list and keeps the selection valid: an unknown previous
// value falls back to the first model.
function applyModels(list: ModelInfo[]) {
  models.value = list
  const current = props.modelValue
  const next = list.some((model) => model.id === current) ? current : (list[0]?.id ?? '')
  if (next !== '' && next !== current) emit('update:modelValue', next)
}

// refresh loads the instance's models. surfaceFailure distinguishes the first
// load (no cache: a failure is shown and gates sending) from a background
// refresh (cache present: a failure is silent).
async function refresh(surfaceFailure: boolean) {
  const token = ++requestToken
  loading.value = true
  try {
    const list = await listInstanceModels(props.projectId, props.instanceId)
    if (token !== requestToken) return
    failed.value = false
    applyModels(list.models)
    rememberModels(props.backend, list.models)
    emit('status', true)
  } catch {
    if (token !== requestToken) return
    if (surfaceFailure) {
      models.value = []
      failed.value = true
      emit('status', false)
    }
  } finally {
    if (token === requestToken) loading.value = false
  }
}

async function load() {
  if (!props.modelSelection || props.instanceId === '') return
  const cached = cachedModels(props.backend)
  if (cached && cached.length > 0) {
    applyModels(cached)
    emit('status', true)
    void refresh(false)
    return
  }
  await refresh(true)
}

onMounted(load)
watch(() => [props.projectId, props.instanceId, props.backend, props.modelSelection], load)

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
      :disabled="loading && models.length === 0"
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

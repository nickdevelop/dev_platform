<script setup lang="ts">
// Model picker for the composer, laid out like the opencode desktop client: a
// compact button shows the current model and opens a searchable list on click,
// instead of an always-visible dropdown. A backend's model list is scoped to the
// project, not to a running instance, so it is available here even when the
// project has no instance yet — creating a session can offer a model.
//
// The picker shows the last list seen for this project and backend the moment it
// mounts, then asks the platform for it. The platform answers from its own cache
// and refreshes behind the response, marking the answer stale; one short retry
// then shows the freshly read list. A failed refresh with a cache present is
// silent: the user already has models and sending stays enabled. With no cache,
// a failure is reported and sending is gated so a free-text model is never used.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { listBackendModels, type BackendId, type ModelInfo } from '../../../api/agents'
import { cachedModels, rememberModels } from '../../../shared/modelCache'

const props = defineProps<{
  projectId: number
  backend: BackendId
  modelSelection: boolean
  modelValue: string
}>()

const emit = defineEmits<{
  'update:modelValue': [id: string]
  // picked fires only when the user chose from the list, so the parent can tell
  // a deliberate choice from the picker's own first-model fallback.
  picked: [id: string]
  status: [ok: boolean]
}>()

const models = ref<ModelInfo[]>([])
const loading = ref(false)
const failed = ref(false)
// A token per request: a response that arrives after a newer one was issued is
// discarded, so switching sessions can never show the previous session's models.
let requestToken = 0

// A stale answer means the platform is re-reading the backend right now. One
// short wait and a bounded number of retries show the new list on their own;
// unbounded polling would turn a slow backend into a stream of requests.
const STALE_RETRY_DELAY = 1200
const STALE_RETRIES = 2
let staleRetries = 0
let staleTimer: ReturnType<typeof setTimeout> | undefined

function scheduleStaleRetry() {
  if (staleRetries >= STALE_RETRIES) return
  staleRetries += 1
  if (staleTimer !== undefined) clearTimeout(staleTimer)
  staleTimer = setTimeout(() => {
    void refresh(false)
  }, STALE_RETRY_DELAY)
}

// Popover state. The list is rendered only while open, which is the point of
// the redesign: the composer stays compact until the user asks for the models.
const open = ref(false)
const query = ref('')
const root = ref<HTMLElement | null>(null)
const searchInput = ref<HTMLInputElement | null>(null)
// Above this many models a search field is worth the space; below it the list
// is short enough to scan directly.
const SEARCH_THRESHOLD = 6

// applyModels shows a list and keeps the selection valid: an unknown previous
// value falls back to the first model.
function applyModels(list: ModelInfo[]) {
  models.value = list
  const current = props.modelValue
  const next = list.some((model) => model.id === current) ? current : (list[0]?.id ?? '')
  if (next !== '' && next !== current) emit('update:modelValue', next)
}

// refresh loads the project's model list for this backend. surfaceFailure
// distinguishes the first load (no cache: a failure is shown and gates sending)
// from a background refresh (cache present: a failure is silent). force asks the
// platform to read the backend now, which is what a caller does when the backend
// has just refused a model the list offered.
async function refresh(surfaceFailure: boolean, force = false) {
  const token = ++requestToken
  loading.value = true
  try {
    const list = await listBackendModels(props.projectId, props.backend, force)
    if (token !== requestToken) return
    failed.value = false
    applyModels(list.models)
    rememberModels(props.projectId, props.backend, list.models)
    emit('status', true)
    if (list.stale) scheduleStaleRetry()
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
  if (!props.modelSelection) return
  if (staleTimer !== undefined) {
    clearTimeout(staleTimer)
    staleTimer = undefined
  }
  staleRetries = 0
  const cached = cachedModels(props.projectId, props.backend)
  if (cached && cached.length > 0) {
    applyModels(cached)
    emit('status', true)
    void refresh(false)
    return
  }
  await refresh(true)
}

// reloadForces re-reads the backend through the platform cache. A parent calls
// it after the backend rejected a model this list offered, so the next choice is
// made from what the backend really runs.
async function reloadForces() {
  if (staleTimer !== undefined) {
    clearTimeout(staleTimer)
    staleTimer = undefined
  }
  staleRetries = 0
  await refresh(true, true)
}

defineExpose({ reloadForces })

onMounted(() => {
  document.addEventListener('mousedown', onDocumentClick)
  void load()
})
onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onDocumentClick)
  if (staleTimer !== undefined) clearTimeout(staleTimer)
})
watch(() => [props.projectId, props.backend, props.modelSelection], load)

// When the parent has no selection for the current session (a session never opened
// before), default to the first model so the picker is never left blank. A session
// that already chose one keeps it: the parent's value survives a session switch.
watch(
  () => props.modelValue,
  (value) => {
    if (props.modelSelection && value === '' && models.value.length > 0) {
      emit('update:modelValue', models.value[0].id)
    }
  },
)

// activeId is the model the composer would send right now: the parent's value
// when it is still in the list, otherwise the first model (the same fallback
// applyModels emits).
const activeId = computed(() => {
  if (props.modelValue) {
    const base = props.modelValue.split('#')[0]
    if (models.value.some((model) => model.id === props.modelValue || model.id === base)) {
      return props.modelValue
    }
  }
  return models.value[0]?.id ?? props.modelValue
})

// Parse base model id and variant from activeId, e.g. "gemini-2.5-flash#thinking"
const parsedSelection = computed(() => {
  const raw = activeId.value || ''
  const hashIndex = raw.indexOf('#')
  if (hashIndex === -1) {
    return { baseId: raw, variant: '' }
  }
  return {
    baseId: raw.slice(0, hashIndex),
    variant: raw.slice(hashIndex + 1),
  }
})

const selected = computed(() => {
  const base = parsedSelection.value.baseId
  return models.value.find((model) => model.id === base || model.id === activeId.value) ?? null
})

const selectedLabel = computed(() => {
  if (selected.value) return selected.value.name || selected.value.id
  if (props.modelValue !== '') return parsedSelection.value.baseId
  return loading.value ? '加载模型…' : '选择模型'
})

const activeVariant = computed(() => parsedSelection.value.variant)
const showSearch = computed(() => models.value.length > SEARCH_THRESHOLD)
const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (q === '') return models.value
  return models.value.filter((model) => {
    const name = (model.name || model.id).toLowerCase()
    return name.includes(q) || model.id.toLowerCase().includes(q)
  })
})

function toggle() {
  if (props.modelSelection && loading.value && models.value.length === 0) return
  open.value = !open.value
  query.value = ''
  if (open.value) void nextTick(() => searchInput.value?.focus())
}

function close() {
  open.value = false
  query.value = ''
}

function choose(id: string) {
  emit('update:modelValue', id)
  emit('picked', id)
  close()
}

// A click anywhere outside the picker dismisses the popover, so the list does
// not linger over the conversation.
function onDocumentClick(event: MouseEvent) {
  if (!open.value) return
  const el = root.value
  if (el && !el.contains(event.target as Node)) close()
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && open.value) {
    event.stopPropagation()
    close()
  }
}
</script>

<template>
  <div v-if="props.modelSelection" ref="root" class="model-select" @keydown="onKeydown">
    <p v-if="failed" class="model-error" role="alert">模型列表加载失败</p>
    <template v-else>
      <button
        type="button"
        class="model-trigger"
        :class="{ open }"
        :disabled="loading && models.length === 0"
        aria-haspopup="listbox"
        :aria-expanded="open"
        aria-label="模型"
        data-test="model-trigger"
        @click="toggle"
      >
        <span class="model-trigger-name">{{ selectedLabel }}</span>
        <span v-if="activeVariant" class="model-variant-badge" :title="`当前变种: ${activeVariant}`">
          {{ activeVariant }}
        </span>
        <span class="model-trigger-caret" aria-hidden="true">▾</span>
      </button>

      <div v-if="open" class="model-menu" role="listbox" aria-label="模型列表" data-test="model-menu">
        <input
          v-if="showSearch"
          ref="searchInput"
          v-model="query"
          class="model-search"
          type="search"
          placeholder="搜索模型"
          aria-label="搜索模型"
        />
        <ul class="model-list">
          <li v-for="model in filtered" :key="model.id" class="model-item">
            <button
              type="button"
              class="model-option"
              :class="{ selected: model.id === parsedSelection.baseId && !activeVariant }"
              role="option"
              :aria-selected="model.id === parsedSelection.baseId && !activeVariant"
              :data-model-id="model.id"
              @click="choose(model.id)"
            >
              <span class="model-option-name">{{ model.name || model.id }}</span>
              <span v-if="model.name" class="model-option-id">{{ model.id }}</span>
            </button>
            <div v-if="model.variants && model.variants.length > 0" class="model-variants">
              <span class="variants-title">变种:</span>
              <button
                v-for="v in model.variants"
                :key="v"
                type="button"
                class="variant-btn"
                :class="{ selected: model.id === parsedSelection.baseId && activeVariant === v }"
                :data-variant="v"
                :title="`选择 ${model.name || model.id} · ${v}`"
                @click.stop="choose(`${model.id}#${v}`)"
              >
                {{ v }}
              </button>
            </div>
          </li>
        </ul>
        <p v-if="filtered.length === 0" class="model-empty">没有匹配的模型</p>
      </div>
    </template>
  </div>
</template>

<style scoped>
.model-select {
  position: relative;
  display: flex;
  align-items: center;
}

/* A compact pill rather than a full-width dropdown: the composer is about
   sending a message, and the model name is a badge the user can open. */
.model-trigger {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  max-width: 16rem;
  height: 28px;
  padding: 0 0.5rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface-muted);
  color: var(--color-text);
  font: inherit;
  font-size: 0.82rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.model-variant-badge {
  display: inline-flex;
  align-items: center;
  padding: 0.08rem 0.32rem;
  border-radius: 4px;
  font-size: 0.7rem;
  line-height: 1;
  font-weight: 500;
  background: var(--color-primary-soft, rgba(59, 130, 246, 0.15));
  color: var(--color-primary, #2563eb);
}

.model-trigger:hover:not(:disabled),
.model-trigger.open {
  border-color: var(--color-primary);
  color: var(--color-primary-hover);
}

.model-trigger:disabled {
  color: var(--color-text-muted);
  cursor: default;
}

.model-trigger-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.model-trigger-caret {
  color: var(--color-text-muted);
  font-size: 0.7rem;
}

.model-menu {
  position: absolute;
  bottom: calc(100% + 0.4rem);
  left: 0;
  z-index: 30;
  display: flex;
  flex-direction: column;
  min-width: 16rem;
  max-width: min(24rem, 80vw);
  max-height: 18rem;
  padding: 0.35rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 10px;
  background: var(--color-surface);
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.2);
  overflow: auto;
}

.model-search {
  margin: 0.1rem 0.1rem 0.35rem;
  padding: 0.3rem 0.5rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface-muted);
  color: var(--color-text);
  font: inherit;
  font-size: 0.85rem;
}

.model-search:focus {
  outline: none;
  border-color: var(--color-primary);
}

.model-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
}

.model-item {
  display: flex;
  flex-direction: column;
}

.model-option {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 0.1rem;
  width: 100%;
  padding: 0.35rem 0.5rem;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--color-text);
  font: inherit;
  font-size: 0.85rem;
  text-align: left;
  cursor: pointer;
}

.model-option:hover,
.model-option:focus-visible {
  background: var(--color-surface-muted);
  outline: none;
}

.model-option.selected {
  background: var(--color-info-soft);
  color: var(--color-info);
}

.model-variants {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.25rem;
  padding: 0.15rem 0.5rem 0.35rem 1rem;
}

.variants-title {
  font-size: 0.72rem;
  color: var(--color-text-muted);
}

.variant-btn {
  display: inline-flex;
  align-items: center;
  padding: 0.1rem 0.35rem;
  border: 1px solid var(--color-border);
  border-radius: 4px;
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
  font-size: 0.72rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.variant-btn:hover {
  border-color: var(--color-primary);
  color: var(--color-primary);
}

.variant-btn.selected {
  border-color: var(--color-primary);
  background: var(--color-primary-soft, rgba(59, 130, 246, 0.15));
  color: var(--color-primary, #2563eb);
  font-weight: 500;
}

.model-option-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 100%;
}

.model-option-id {
  color: var(--color-text-muted);
  font-size: 0.72rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 100%;
}

.model-empty {
  margin: 0.35rem 0.5rem;
  color: var(--color-text-muted);
  font-size: 0.8rem;
}

.model-error {
  margin: 0;
  color: var(--color-danger);
  font-size: 0.8rem;
}
</style>

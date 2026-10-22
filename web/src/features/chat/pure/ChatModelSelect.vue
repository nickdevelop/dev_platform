<script setup lang="ts">
// Model picker for pure chat. It lists the text models the operator enabled in
// Platform Settings and emits the model-definition id the chat API expects.
// The agent composer's backend-scoped picker is intentionally not reused: chat
// has no project or backend.
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useModelsStore } from '../../../stores/models'

const props = defineProps<{
  modelValue: number | null
  disabled?: boolean
  fallbackLabel?: string
}>()

const emit = defineEmits<{ 'update:modelValue': [id: number] }>()

const models = useModelsStore()
const open = ref(false)
const query = ref('')
const root = ref<HTMLElement | null>(null)
const searchInput = ref<HTMLInputElement | null>(null)

const options = computed(() =>
  models.services.flatMap((service) =>
    service.models
      .filter((model) => model.usable && model.supportsText)
      .map((model) => ({
        id: model.id,
        name: model.name || model.modelId,
        wireId: model.modelId,
        serviceName: service.name,
        supportsImage: model.supportsImage,
      })),
  ),
)

const showSearch = computed(() => options.value.length > 6)
const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (q === '') return options.value
  return options.value.filter((option) =>
    `${option.name} ${option.wireId} ${option.serviceName}`.toLowerCase().includes(q),
  )
})

const selected = computed(() => options.value.find((option) => option.id === props.modelValue) ?? null)
const selectedLabel = computed(() => {
  if (selected.value) return selected.value.name
  if (props.fallbackLabel) return props.fallbackLabel
  if (models.loading && options.value.length === 0) return '加载模型…'
  return '选择模型'
})

function toggle() {
  if (props.disabled) return
  open.value = !open.value
  query.value = ''
  if (open.value) void nextTick(() => searchInput.value?.focus())
}

function choose(id: number) {
  emit('update:modelValue', id)
  open.value = false
  query.value = ''
}

function onDocumentClick(event: MouseEvent) {
  if (!open.value) return
  if (root.value && !root.value.contains(event.target as Node)) open.value = false
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && open.value) {
    event.stopPropagation()
    open.value = false
  }
}

onMounted(() => {
  document.addEventListener('mousedown', onDocumentClick)
  if (models.services.length === 0) void models.load()
})
onBeforeUnmount(() => document.removeEventListener('mousedown', onDocumentClick))
</script>

<template>
  <div ref="root" class="chat-model-select composer-select" @keydown="onKeydown">
    <button
      type="button"
      class="chat-model-trigger composer-select-trigger"
      :class="{ open }"
      :disabled="props.disabled || options.length === 0"
      aria-haspopup="listbox"
      :aria-expanded="open"
      aria-label="模型"
      data-test="chat-model-trigger"
      @click="toggle"
    >
      <span class="chat-model-name">{{ selectedLabel }}</span>
      <span class="composer-select-caret" aria-hidden="true">▾</span>
    </button>

    <div v-if="open" class="chat-model-menu composer-select-menu" role="listbox" aria-label="模型列表" data-test="chat-model-menu">
      <input
        v-if="showSearch"
        ref="searchInput"
        v-model="query"
        class="chat-model-search"
        type="search"
        placeholder="搜索模型"
        aria-label="搜索模型"
      />
      <p v-if="options.length === 0" class="chat-model-empty">尚未配置可用模型</p>
      <button
        v-for="option in filtered"
        :key="option.id"
        type="button"
        class="chat-model-option composer-select-option"
        :class="{ selected: option.id === props.modelValue }"
        role="option"
        :aria-selected="option.id === props.modelValue"
        :data-model-id="option.id"
        @click="choose(option.id)"
      >
        <span class="chat-model-option-name">{{ option.name }}</span>
        <span class="chat-model-option-meta">
          {{ option.serviceName }} · {{ option.wireId }}
          <template v-if="option.supportsImage"> · 图片</template>
        </span>
      </button>
    </div>
  </div>
</template>

<style scoped src="../composer/composer-select.css"></style>

<style scoped>
.chat-model-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chat-model-menu {
  bottom: calc(100% + 0.4rem);
}

.chat-model-search {
  width: 100%;
  box-sizing: border-box;
  padding: 0.35rem 0.5rem;
  margin-bottom: 0.25rem;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
  font-size: 0.82rem;
}

.chat-model-empty {
  margin: 0;
  padding: 0.5rem;
  color: var(--color-text-muted);
  font-size: 0.82rem;
}

.chat-model-option-name {
  font-size: 0.85rem;
}

.chat-model-option-meta {
  color: var(--color-text-muted);
  font-size: 0.72rem;
}

.chat-model-option.selected {
  background: var(--color-info-soft);
}
</style>

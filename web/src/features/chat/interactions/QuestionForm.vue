<script setup lang="ts">
// Generic renderer for an opencode question form (Form.Info). Fields stay
// opaque: the Form.Field union is rendered by type, so new field kinds degrade
// to a text input instead of breaking. Answers are emitted as Form.Answer
// (Record<key, string|number|boolean|string[]>).
import { computed, reactive, watch } from 'vue'
import { deriveFieldRecommendedValue } from './autoApproval'
import type { FormField, FormOption } from './types'

export type { FormField, FormOption }

const props = defineProps<{
  formId: string
  title: string
  fields: unknown
  busy?: boolean
  autopilot?: boolean
}>()

const emit = defineEmits<{
  submit: [answer: Record<string, unknown>]
}>()

// Normalize the opaque fields payload into renderable fields. Hidden fields
// are skipped: the server falls back to their defaults.
const visibleFields = computed<FormField[]>(() => {
  if (!Array.isArray(props.fields)) return []
  return (props.fields as FormField[]).filter((field) => field && field.key && !field.hidden)
})

function defaultFor(field: FormField): unknown {
  if (props.autopilot) {
    const rec = deriveFieldRecommendedValue(field)
    if (rec !== undefined) return rec
  }
  if (field.default !== undefined) return field.default
  switch (field.type) {
    case 'boolean':
      return null
    case 'multiselect':
      return []
    default:
      return ''
  }
}

const values = reactive<Record<string, unknown>>(
  Object.fromEntries(visibleFields.value.map((field) => [field.key, defaultFor(field)])),
)

watch(
  () => props.autopilot,
  (enabled) => {
    if (!enabled) return
    for (const field of visibleFields.value) {
      if (!isAnswered(field)) {
        const rec = deriveFieldRecommendedValue(field)
        if (rec !== undefined) values[field.key] = rec
      }
    }
  },
)

function fieldTitle(field: FormField): string {
  return field.title || field.key
}

function isAnswered(field: FormField): boolean {
  const value = values[field.key]
  if (Array.isArray(value)) return value.length > 0
  return value !== '' && value !== null && value !== undefined
}

// Only required visible fields gate the submit: an optional question may stay
// unanswered and the server applies its default.
const canSubmit = computed(
  () =>
    !props.busy &&
    visibleFields.value.length > 0 &&
    visibleFields.value.every((field) => !field.required || isAnswered(field)),
)

function toggleMultiselect(field: FormField, value: string) {
  const current = values[field.key]
  const list = Array.isArray(current) ? [...(current as unknown[])] : []
  const at = list.indexOf(value)
  if (at >= 0) list.splice(at, 1)
  else list.push(value)
  values[field.key] = list
}

function submit() {
  if (!canSubmit.value) return
  const answer: Record<string, unknown> = {}
  for (const field of visibleFields.value) {
    const value = values[field.key]
    if (value === '' || value === null || value === undefined) continue
    if (Array.isArray(value) && value.length === 0) continue
    if (field.type === 'number' || field.type === 'integer') {
      const numeric = typeof value === 'number' ? value : Number(value)
      if (Number.isNaN(numeric)) continue
      answer[field.key] = numeric
      continue
    }
    answer[field.key] = value
  }
  emit('submit', answer)
}
</script>

<template>
  <section class="question-form" data-test="question-form" :data-form-id="props.formId" aria-label="提问">
    <p class="question-title">{{ props.title }}</p>
    <div v-for="field in visibleFields" :key="field.key" class="field">
      <p class="field-title">
        {{ fieldTitle(field) }}
        <span v-if="field.required" class="required">*</span>
      </p>
      <p v-if="field.description" class="field-description">{{ field.description }}</p>

      <template v-if="field.type === 'string' && field.options?.length">
        <label v-for="option in field.options" :key="option.value" class="option">
          <input
            type="radio"
            :name="`${props.formId}:${field.key}`"
            :value="option.value"
            v-model="values[field.key]"
            :disabled="props.busy"
          />
          <span class="option-label">{{ option.label }}</span>
          <span v-if="option.description" class="option-description">{{ option.description }}</span>
        </label>
        <input
          v-if="field.custom"
          v-model="values[field.key]"
          class="text-input"
          type="text"
          placeholder="或自定义输入…"
          :disabled="props.busy"
        />
      </template>

      <div v-else-if="field.type === 'boolean'" class="boolean-options">
        <button type="button" :class="{ selected: values[field.key] === true }" :disabled="props.busy" @click="values[field.key] = true">是</button>
        <button type="button" :class="{ selected: values[field.key] === false }" :disabled="props.busy" @click="values[field.key] = false">否</button>
      </div>

      <div v-else-if="field.type === 'multiselect'" class="multiselect-options">
        <label v-for="option in field.options ?? []" :key="option.value" class="option">
          <input
            type="checkbox"
            :checked="(values[field.key] as unknown[]).includes(option.value)"
            :disabled="props.busy"
            @change="toggleMultiselect(field, option.value)"
          />
          <span class="option-label">{{ option.label }}</span>
        </label>
      </div>

      <input
        v-else-if="field.type === 'number' || field.type === 'integer'"
        v-model="values[field.key]"
        class="text-input"
        type="number"
        :disabled="props.busy"
      />

      <template v-else>
        <a v-if="field.type === 'external' && field.url" :href="field.url" target="_blank" rel="noreferrer" class="external-link">在外部页面中查看</a>
        <input v-model="values[field.key]" class="text-input" type="text" :disabled="props.busy" />
      </template>
    </div>
    <p v-if="visibleFields.length === 0" class="no-fields">该提问没有可填写的字段。</p>
    <button type="button" class="submit-answer" data-test="submit-answer" :disabled="!canSubmit" @click="submit">
      {{ props.busy ? '提交中…' : '提交回答' }}
    </button>
  </section>
</template>

<style scoped>
.question-form {
  border: 1px solid var(--color-border-strong);
  border-radius: 8px;
  background: var(--color-surface);
  padding: 0.6rem 0.8rem;
  margin: 0.5rem 0;
}

.question-title {
  margin: 0 0 0.4rem;
  font-weight: 600;
}

.field {
  margin: 0.5rem 0;
}

.field-title {
  margin: 0;
  font-weight: 600;
  font-size: 0.9rem;
}

.required {
  color: var(--color-danger);
}

.field-description {
  margin: 0.15rem 0 0.3rem;
  color: var(--color-text-muted);
  font-size: 0.85rem;
}

.option {
  display: flex;
  align-items: baseline;
  gap: 0.4rem;
  padding: 0.25rem 0;
  cursor: pointer;
}

.option-label {
  font-weight: 600;
}

.option-description {
  color: var(--color-text-muted);
  font-size: 0.85rem;
}

.text-input {
  width: 100%;
  box-sizing: border-box;
  padding: 0.35rem 0.5rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  font: inherit;
  margin-top: 0.3rem;
}

.boolean-options,
.multiselect-options {
  display: flex;
  gap: 0.4rem;
  flex-wrap: wrap;
}

.boolean-options button {
  padding: 0.3rem 0.8rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  cursor: pointer;
  font: inherit;
}

.boolean-options button.selected {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: var(--color-surface);
}

.external-link {
  font-size: 0.85rem;
}

.no-fields {
  color: var(--color-text-muted);
  font-size: 0.85rem;
}

.submit-answer {
  margin-top: 0.4rem;
  padding: 0.35rem 0.8rem;
  border: 1px solid var(--color-primary);
  border-radius: 6px;
  background: var(--color-primary);
  color: var(--color-surface);
  cursor: pointer;
  font: inherit;
}

.submit-answer:disabled {
  background: var(--color-info-border);
  border-color: var(--color-info-border);
  cursor: default;
}
</style>

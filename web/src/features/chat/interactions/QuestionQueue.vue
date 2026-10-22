<script setup lang="ts">
// Stepwise question queue for opencode forms. Instead of rendering every
// pending form (and every field of each form) at once — which overflows when
// the agent asks many questions — the queue shows one form at a time and one
// field of that form at a time, like the opencode desktop client: answer the
// current question, move to the next, submit when the form's questions are
// done, then continue with the next pending form.
import { computed, reactive, ref, watch } from 'vue'
import type { FormInfo } from '../../../api/interactions'
import type { FormField } from './types'
import { deriveFieldRecommendedValue } from './autoApproval'

const props = defineProps<{
  forms: FormInfo[]
  busy?: boolean
  autopilot?: boolean
}>()

const emit = defineEmits<{
  submit: [formId: string, answer: Record<string, unknown>]
}>()

const activeIndex = ref(0)
const stepIndex = ref(0)
const values = reactive<Record<string, Record<string, unknown>>>({})

function fieldsOf(form: FormInfo): FormField[] {
  if (!Array.isArray(form.fields)) return []
  return (form.fields as FormField[]).filter((field) => field && field.key && !field.hidden)
}

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

const activeForm = computed<FormInfo | null>(() => {
  if (props.forms.length === 0) return null
  return props.forms[Math.min(activeIndex.value, props.forms.length - 1)] ?? null
})

const activeFields = computed<FormField[]>(() => (activeForm.value ? fieldsOf(activeForm.value) : []))

const stepField = computed<FormField | null>(() => {
  const fields = activeFields.value
  if (fields.length === 0) return null
  return fields[Math.min(stepIndex.value, fields.length - 1)] ?? null
})

function valuesFor(formId: string): Record<string, unknown> {
  if (!values[formId]) values[formId] = {}
  const bucket = values[formId]
  for (const field of fieldsOf(props.forms.find((entry) => entry.id === formId) ?? { id: formId, sessionId: '', title: '', fields: [] })) {
    if (!(field.key in bucket)) bucket[field.key] = defaultFor(field)
  }
  return bucket
}

function fieldTitle(field: FormField): string {
  return field.title || field.key
}

function isAnswered(field: FormField, bucket: Record<string, unknown>): boolean {
  const value = bucket[field.key]
  if (Array.isArray(value)) return value.length > 0
  return value !== '' && value !== null && value !== undefined
}

const canAdvance = computed(() => {
  const form = activeForm.value
  const field = stepField.value
  if (!form || !field) return false
  if (props.busy) return false
  const bucket = valuesFor(form.id)
  return !field.required || isAnswered(field, bucket)
})

const isLastStep = computed(() => stepIndex.value >= activeFields.value.length - 1)

const canSubmit = computed(() => {
  const form = activeForm.value
  if (!form || props.busy) return false
  const fields = activeFields.value
  if (fields.length === 0) return false
  const bucket = valuesFor(form.id)
  if (!isLastStep.value) return false
  return fields.every((field) => !field.required || isAnswered(field, bucket))
})

function toggleMultiselect(formId: string, field: FormField, value: string) {
  const bucket = valuesFor(formId)
  const current = bucket[field.key]
  const list = Array.isArray(current) ? [...(current as unknown[])] : []
  const at = list.indexOf(value)
  if (at >= 0) list.splice(at, 1)
  else list.push(value)
  bucket[field.key] = list
}

function next() {
  if (!canAdvance.value) return
  if (!isLastStep.value) stepIndex.value += 1
}

function prev() {
  if (stepIndex.value > 0) stepIndex.value -= 1
}

function submit() {
  const form = activeForm.value
  if (!form || !canSubmit.value) return
  const bucket = valuesFor(form.id)
  const answer: Record<string, unknown> = {}
  for (const field of activeFields.value) {
    const value = bucket[field.key]
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
  emit('submit', form.id, answer)
}

// When the pending list changes (answered, expired, new question), clamp the
// indices and reset the step when the active form changes.
watch(
  () => props.forms.map((entry) => entry.id).join(','),
  () => {
    if (activeIndex.value >= props.forms.length) activeIndex.value = 0
    stepIndex.value = 0
  },
)

watch(activeForm, () => {
  stepIndex.value = 0
})

watch(
  () => props.autopilot,
  (enabled) => {
    if (!enabled) return
    for (const form of props.forms) {
      const bucket = valuesFor(form.id)
      for (const field of fieldsOf(form)) {
        if (!isAnswered(field, bucket)) {
          const rec = deriveFieldRecommendedValue(field)
          if (rec !== undefined) bucket[field.key] = rec
        }
      }
    }
  },
)
</script>

<template>
  <section
    v-if="activeForm"
    class="question-queue"
    data-test="question-form"
    :data-form-id="activeForm.id"
    aria-label="提问"
  >
    <div class="queue-head">
      <p class="queue-title">{{ activeForm.title }}</p>
      <p class="queue-progress" data-test="question-progress">
        <template v-if="props.forms.length > 1">表单 {{ activeIndex + 1 }}/{{ props.forms.length }} · </template>
        问题 {{ activeFields.length === 0 ? 0 : stepIndex + 1 }}/{{ activeFields.length }}
      </p>
    </div>
    <div v-if="props.forms.length > 1" class="queue-tabs" role="tablist" aria-label="待确认的提问">
      <button
        v-for="(form, index) in props.forms"
        :key="form.id"
        type="button"
        class="queue-tab"
        :class="{ active: index === Math.min(activeIndex, props.forms.length - 1) }"
        :disabled="props.busy"
        @click="activeIndex = index; stepIndex = 0"
      >
        {{ index + 1 }}
      </button>
    </div>

    <div v-if="stepField && activeForm" class="field">
      <p class="field-title">
        {{ fieldTitle(stepField) }}
        <span v-if="stepField.required" class="required">*</span>
      </p>
      <p v-if="stepField.description" class="field-description">{{ stepField.description }}</p>

      <template v-if="stepField.type === 'string' && stepField.options?.length">
        <label v-for="option in stepField.options" :key="option.value" class="option">
          <input
            type="radio"
            :name="`${activeForm.id}:${stepField.key}`"
            :value="option.value"
            v-model="valuesFor(activeForm.id)[stepField.key]"
            :disabled="props.busy"
          />
          <span class="option-label">{{ option.label }}</span>
          <span v-if="option.description" class="option-description">{{ option.description }}</span>
        </label>
        <input
          v-if="stepField.custom"
          v-model="valuesFor(activeForm.id)[stepField.key]"
          class="text-input"
          type="text"
          placeholder="或自定义输入…"
          :disabled="props.busy"
        />
      </template>

      <div v-else-if="stepField.type === 'boolean'" class="boolean-options">
        <button type="button" :class="{ selected: valuesFor(activeForm.id)[stepField.key] === true }" :disabled="props.busy" @click="valuesFor(activeForm.id)[stepField.key] = true">是</button>
        <button type="button" :class="{ selected: valuesFor(activeForm.id)[stepField.key] === false }" :disabled="props.busy" @click="valuesFor(activeForm.id)[stepField.key] = false">否</button>
      </div>

      <div v-else-if="stepField.type === 'multiselect'" class="multiselect-options">
        <label v-for="option in stepField.options ?? []" :key="option.value" class="option">
          <input
            type="checkbox"
            :checked="(valuesFor(activeForm.id)[stepField.key] as unknown[] ?? []).includes(option.value)"
            :disabled="props.busy"
            @change="toggleMultiselect(activeForm.id, stepField!, option.value)"
          />
          <span class="option-label">{{ option.label }}</span>
        </label>
      </div>

      <input
        v-else-if="stepField.type === 'number' || stepField.type === 'integer'"
        v-model="valuesFor(activeForm.id)[stepField.key]"
        class="text-input"
        type="number"
        :disabled="props.busy"
      />

      <template v-else>
        <a v-if="stepField.type === 'external' && stepField.url" :href="stepField.url" target="_blank" rel="noreferrer" class="external-link">在外部页面中查看</a>
        <input v-model="valuesFor(activeForm.id)[stepField.key]" class="text-input" type="text" :disabled="props.busy" />
      </template>
    </div>
    <p v-else class="no-fields">该提问没有可填写的字段。</p>

    <div class="queue-actions">
      <button
        type="button"
        class="nav-button"
        data-test="question-prev"
        :disabled="stepIndex === 0 || props.busy"
        @click="prev"
      >
        上一题
      </button>
      <button
        v-if="!isLastStep"
        type="button"
        class="nav-button primary"
        data-test="question-next"
        :disabled="!canAdvance"
        @click="next"
      >
        下一题
      </button>
      <button
        v-else
        type="button"
        class="submit-answer"
        data-test="submit-answer"
        :disabled="!canSubmit"
        @click="submit"
      >
        {{ props.busy ? '提交中…' : '提交回答' }}
      </button>
    </div>
  </section>
</template>

<style scoped>
.question-queue {
  border: 1px solid var(--color-border-strong);
  border-radius: 8px;
  background: var(--color-surface);
  padding: 0.6rem 0.8rem;
  margin: 0.5rem 0;
}

.queue-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.5rem;
}

.queue-title {
  margin: 0;
  font-weight: 600;
}

.queue-progress {
  margin: 0;
  color: var(--color-text-muted);
  font-size: 0.8rem;
  white-space: nowrap;
}

.queue-tabs {
  display: flex;
  gap: 0.3rem;
  margin-top: 0.4rem;
}

.queue-tab {
  width: 1.6rem;
  height: 1.6rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 999px;
  background: var(--color-surface);
  cursor: pointer;
  font: inherit;
  font-size: 0.8rem;
  color: var(--color-text-muted);
}

.queue-tab.active {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: var(--color-surface);
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

.queue-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
  margin-top: 0.5rem;
}

.nav-button {
  padding: 0.35rem 0.8rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  cursor: pointer;
  font: inherit;
}

.nav-button.primary {
  background: var(--color-surface-muted);
}

.nav-button:disabled {
  cursor: default;
  opacity: 0.5;
}

.submit-answer {
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

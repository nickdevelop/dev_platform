<script setup lang="ts" generic="T = string | number | null">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { SelectMenuItem } from './selectMenuTypes'

defineOptions({
  inheritAttrs: false,
})

const props = withDefaults(
  defineProps<{
    modelValue: T
    options: SelectMenuItem<T>[]
    placeholder?: string
    searchable?: boolean
    clearable?: boolean
    size?: 'sm' | 'md'
    block?: boolean
    disabled?: boolean
    placement?: 'bottom' | 'top'
    id?: string
  }>(),
  {
    placeholder: '请选择…',
    searchable: undefined,
    clearable: false,
    size: 'md',
    block: false,
    disabled: false,
    placement: 'bottom',
    id: undefined,
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: T]
  change: [value: T]
  clear: []
}>()

const isOpen = ref(false)
const query = ref('')
const rootRef = ref<HTMLElement | null>(null)
const searchInputRef = ref<HTMLInputElement | null>(null)

const SEARCH_THRESHOLD = 6

const shouldShowSearch = computed(() => {
  if (props.searchable !== undefined) return props.searchable
  return props.options.length > SEARCH_THRESHOLD
})

const selectedOption = computed(() => {
  return props.options.find((opt) => String(opt.value) === String(props.modelValue)) ?? null
})

const filteredOptions = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return props.options
  return props.options.filter((opt) => {
    const labelMatch = (opt.label || '').toLowerCase().includes(q)
    const subMatch = (opt.sublabel || '').toLowerCase().includes(q)
    const tagMatch = (opt.tag || '').toLowerCase().includes(q)
    return labelMatch || subMatch || tagMatch
  })
})

function toggle() {
  if (props.disabled) return
  isOpen.value = !isOpen.value
  query.value = ''
  if (isOpen.value && shouldShowSearch.value) {
    void nextTick(() => {
      searchInputRef.value?.focus()
    })
  }
}

function close() {
  isOpen.value = false
  query.value = ''
}

function selectOption(option: SelectMenuItem<T>) {
  if (option.disabled) return
  emit('update:modelValue', option.value)
  emit('change', option.value)
  close()
}

function handleClear(e: MouseEvent) {
  e.stopPropagation()
  emit('update:modelValue', '' as unknown as T)
  emit('change', '' as unknown as T)
  emit('clear')
}

function onNativeSelectChange(event: Event) {
  const target = event.target as HTMLSelectElement
  const rawValue = target.value
  const matched = props.options.find((opt) => String(opt.value) === rawValue)
  const val = matched ? matched.value : (rawValue as unknown as T)
  emit('update:modelValue', val)
  emit('change', val)
}

function onDocumentClick(e: MouseEvent) {
  if (!isOpen.value) return
  const el = rootRef.value
  if (el && !el.contains(e.target as Node)) {
    close()
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && isOpen.value) {
    e.stopPropagation()
    close()
  }
}

onMounted(() => {
  document.addEventListener('mousedown', onDocumentClick)
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onDocumentClick)
})

watch(
  () => props.disabled,
  (disabled) => {
    if (disabled && isOpen.value) close()
  },
)
</script>

<template>
  <div
    ref="rootRef"
    class="ui-select-menu"
    :class="[
      block ? 'w-full' : 'inline-block',
      size === 'sm' ? 'size-sm' : 'size-md',
      { disabled, 'is-open': isOpen },
    ]"
    @keydown="onKeydown"
  >
    <!-- Hidden native select for form accessibility, automated test compatibility & semantics -->
    <select
      :id="id"
      :value="modelValue ?? ''"
      :disabled="disabled"
      tabindex="-1"
      aria-hidden="true"
      class="hidden-native-select"
      v-bind="$attrs"
      @change="onNativeSelectChange"
    >
      <option value="" disabled>{{ placeholder }}</option>
      <option
        v-for="opt in options"
        :key="String(opt.value)"
        :value="String(opt.value)"
        :disabled="opt.disabled"
      >
        {{ opt.label }}
      </option>
    </select>

    <!-- Trigger Button -->
    <button
      type="button"
      class="select-trigger"
      :class="{ open: isOpen, 'has-value': !!selectedOption }"
      :disabled="disabled"
      aria-haspopup="listbox"
      :aria-expanded="isOpen"
      @click="toggle"
    >
      <slot name="trigger" :selected="selectedOption" :is-open="isOpen">
        <div class="trigger-content">
          <span v-if="selectedOption" class="selected-label">
            {{ selectedOption.label }}
          </span>
          <span v-if="selectedOption?.sublabel" class="selected-sublabel">
            {{ selectedOption.sublabel }}
          </span>
          <span v-if="!selectedOption" class="placeholder-label">
            {{ placeholder }}
          </span>
        </div>
      </slot>

      <span v-if="selectedOption?.tag" class="selected-tag">
        {{ selectedOption.tag }}
      </span>

      <span
        v-if="clearable && selectedOption && !disabled"
        class="clear-btn inline-flex items-center justify-center"
        role="button"
        title="清除选择"
        tabindex="0"
        @click="handleClear"
        @keydown.enter.prevent="handleClear"
        @keydown.space.prevent="handleClear"
      >
        <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="1.8">
          <line x1="3" y1="3" x2="13" y2="13" />
          <line x1="13" y1="3" x2="3" y2="13" />
        </svg>
      </span>

      <span class="trigger-caret" aria-hidden="true">▾</span>
    </button>

    <!-- Popover Menu -->
    <Transition
      enter-active-class="transition ease-out duration-100"
      enter-from-class="transform opacity-0 scale-95"
      enter-to-class="transform opacity-100 scale-100"
      leave-active-class="transition ease-in duration-75"
      leave-from-class="transform opacity-100 scale-100"
      leave-to-class="transform opacity-0 scale-95"
    >
      <div
        v-if="isOpen"
        class="select-menu-popover"
        :class="[
          placement === 'top' ? 'placement-top' : 'placement-bottom',
          block ? 'w-full' : '',
        ]"
        role="listbox"
      >
        <!-- Search Input -->
        <div v-if="shouldShowSearch" class="search-box">
          <input
            ref="searchInputRef"
            v-model="query"
            type="search"
            class="search-input"
            placeholder="快速搜索…"
            aria-label="快速搜索"
            @click.stop
          />
        </div>

        <!-- Options List -->
        <ul class="options-list">
          <li
            v-for="opt in filteredOptions"
            :key="String(opt.value)"
            class="option-item"
            :class="{
              selected: String(opt.value) === String(modelValue),
              disabled: opt.disabled,
            }"
            role="option"
            :aria-selected="String(opt.value) === String(modelValue)"
            @click="selectOption(opt)"
          >
            <slot name="option" :option="opt" :is-selected="String(opt.value) === String(modelValue)">
              <div class="option-main">
                <span class="option-label">{{ opt.label }}</span>
                <span v-if="opt.sublabel" class="option-sublabel">{{ opt.sublabel }}</span>
              </div>
              <span v-if="opt.tag" class="option-tag">{{ opt.tag }}</span>
              <span v-if="String(opt.value) === String(modelValue)" class="check-icon inline-flex items-center justify-center" aria-hidden="true">
                <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2">
                  <polyline points="3 8 6 11 13 4" />
                </svg>
              </span>
            </slot>
          </li>

          <li v-if="filteredOptions.length === 0" class="empty-hint">
            没有匹配的选项
          </li>
        </ul>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.ui-select-menu {
  position: relative;
  font-family: inherit;
}

.hidden-native-select {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border-width: 0;
}

.select-trigger {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 0.4rem;
  border: 1px solid var(--color-border-strong, #cbd5e1);
  border-radius: var(--radius, 6px);
  background: var(--color-surface, #ffffff);
  color: var(--color-text, #0f172a);
  font: inherit;
  font-size: 0.857rem;
  text-align: left;
  cursor: pointer;
  transition: all 0.15s ease;
  box-sizing: border-box;
}

.size-sm .select-trigger {
  height: 28px;
  padding: 0 0.5rem;
  font-size: 0.8rem;
}

.size-md .select-trigger {
  height: 34px;
  padding: 0 0.65rem;
  font-size: 0.857rem;
}

.select-trigger:hover:not(:disabled),
.select-trigger.open {
  border-color: var(--color-primary, #2563eb);
  box-shadow: 0 0 0 1px var(--color-primary, #2563eb);
}

.select-trigger:disabled {
  opacity: 0.55;
  cursor: not-allowed;
  background: var(--color-surface-muted, #f8fafc);
}

.trigger-content {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: baseline;
  gap: 0.45rem;
  overflow: hidden;
}

.selected-label {
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.selected-sublabel {
  font-size: 0.78rem;
  color: var(--color-text-muted, #64748b);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.placeholder-label {
  color: var(--color-text-muted, #94a3b8);
}

.selected-tag {
  display: inline-flex;
  align-items: center;
  padding: 0.1rem 0.35rem;
  border-radius: 4px;
  font-size: 0.7rem;
  font-weight: 500;
  background: var(--color-primary-soft, rgba(37, 99, 235, 0.1));
  color: var(--color-primary, #2563eb);
  flex-shrink: 0;
}

.clear-btn {
  font-size: 0.72rem;
  padding: 0.1rem 0.25rem;
  color: var(--color-text-muted, #94a3b8);
  border-radius: 3px;
  transition: all 0.15s ease;
}

.clear-btn:hover {
  background: var(--color-surface-muted, #f1f5f9);
  color: var(--color-text, #0f172a);
}

.trigger-caret {
  color: var(--color-text-muted, #94a3b8);
  font-size: 0.72rem;
  transition: transform 0.15s ease;
  flex-shrink: 0;
}

.select-trigger.open .trigger-caret {
  transform: rotate(180deg);
}

/* Popover Menu */
.select-menu-popover {
  position: absolute;
  left: 0;
  z-index: 50;
  min-width: 100%;
  max-width: min(28rem, 90vw);
  max-height: 18rem;
  display: flex;
  flex-direction: column;
  padding: 0.35rem;
  border: 1px solid var(--color-border-strong, #cbd5e1);
  border-radius: var(--radius, 8px);
  background: var(--color-surface, #ffffff);
  box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.15), 0 8px 10px -6px rgba(0, 0, 0, 0.1);
  box-sizing: border-box;
}

.placement-bottom {
  top: calc(100% + 4px);
}

.placement-top {
  bottom: calc(100% + 4px);
}

.search-box {
  padding: 0.15rem 0.15rem 0.35rem;
  flex-shrink: 0;
}

.search-input {
  width: 100%;
  padding: 0.3rem 0.5rem;
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 5px;
  background: var(--color-surface-muted, #f8fafc);
  color: var(--color-text, #0f172a);
  font: inherit;
  font-size: 0.82rem;
  box-sizing: border-box;
  outline: none;
  transition: border-color 0.15s ease;
}

.search-input:focus {
  border-color: var(--color-primary, #2563eb);
}

.options-list {
  list-style: none;
  margin: 0;
  padding: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.option-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0.4rem 0.55rem;
  border-radius: 5px;
  background: transparent;
  color: var(--color-text, #0f172a);
  font-size: 0.857rem;
  cursor: pointer;
  transition: all 0.12s ease;
  user-select: none;
}

.option-item:hover:not(.disabled) {
  background: var(--color-surface-muted, #f1f5f9);
}

.option-item.selected {
  background: var(--color-primary-soft, rgba(37, 99, 235, 0.1));
  color: var(--color-primary, #2563eb);
  font-weight: 500;
}

.option-item.disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.option-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 0.1rem;
}

.option-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.option-sublabel {
  font-size: 0.75rem;
  color: var(--color-text-muted, #64748b);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.option-tag {
  font-size: 0.7rem;
  padding: 0.05rem 0.3rem;
  border-radius: 3px;
  background: var(--color-surface-muted, #f1f5f9);
  color: var(--color-text-muted, #64748b);
  flex-shrink: 0;
}

.check-icon {
  font-size: 0.8rem;
  font-weight: bold;
  color: var(--color-primary, #2563eb);
  flex-shrink: 0;
}

.empty-hint {
  padding: 0.75rem;
  text-align: center;
  color: var(--color-text-muted, #94a3b8);
  font-size: 0.8rem;
}
</style>

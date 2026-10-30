<script setup lang="ts">
import { nextTick, watch, ref } from 'vue'
import type { DropdownItem } from './types'

const props = defineProps<{
  items: readonly DropdownItem[]
  selectedIndex: number
  loading?: boolean
}>()

const emit = defineEmits<{
  select: [index: number]
}>()

const listRef = ref<HTMLElement | null>(null)

watch(
  () => props.selectedIndex,
  async (newIndex) => {
    await nextTick()
    if (!listRef.value) return
    const activeEl = listRef.value.querySelector<HTMLElement>(`[data-index="${newIndex}"]`)
    if (activeEl) {
      activeEl.scrollIntoView({ block: 'nearest' })
    }
  },
)
</script>

<template>
  <div class="composer-dropdown" role="listbox" aria-label="自动补全建议">
    <div v-if="loading && items.length === 0" class="dropdown-loading">正在加载建议…</div>
    <div v-else ref="listRef" class="dropdown-list">
      <div
        v-for="(item, index) in items"
        :key="item.id"
        class="dropdown-item"
        :class="{ active: index === selectedIndex }"
        :data-index="index"
        role="option"
        :aria-selected="index === selectedIndex"
        @mousedown.prevent="emit('select', index)"
      >
        <div class="item-main">
          <span class="item-icon" aria-hidden="true">
            <template v-if="item.kind === 'command'">
              <svg class="glyph-icon" viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8">
                <polyline points="4 5 8 8 4 11" />
                <line x1="9" y1="11" x2="13" y2="11" />
              </svg>
            </template>
            <template v-else-if="item.kind === 'file'">
              <svg class="glyph-icon" viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5">
                <path d="M3 2h7l3 3v9H3V2z" />
                <polyline points="10 2 10 5 13 5" />
              </svg>
            </template>
            <template v-else-if="item.kind === 'folder'">
              <svg class="glyph-icon" viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5">
                <path d="M2 3h4l2 2h6v8H2V3z" />
              </svg>
            </template>
            <template v-else-if="item.kind === 'session'">
              <svg class="glyph-icon" viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5">
                <path d="M2 4h12v7H5l-3 3V4z" />
              </svg>
            </template>
            <template v-else>•</template>
          </span>
          <span class="item-label">{{ item.label }}</span>
          <span v-if="item.detail" class="item-detail">{{ item.detail }}</span>
        </div>
        <span v-if="item.badge" class="item-badge">{{ item.badge }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.composer-dropdown {
  position: absolute;
  bottom: calc(100% + 0.35rem);
  left: 0;
  right: 0;
  z-index: 50;
  max-height: 16rem;
  background: var(--color-surface, #15181f);
  border: 1px solid var(--color-border-strong, #2e3545);
  border-radius: var(--radius, 4px);
  overflow-y: auto;
  padding: 0.25rem;
}

.dropdown-loading {
  padding: 0.5rem 0.75rem;
  color: var(--color-text-muted, #8a8c92);
  font-size: 0.82rem;
  text-align: center;
}

.dropdown-list {
  display: flex;
  flex-direction: column;
  gap: 0.1rem;
}

/* Compact single-row layout */
.dropdown-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0.25rem 0.5rem;
  min-height: 28px;
  border-radius: 2px;
  cursor: pointer;
  border-left: 2px solid transparent;
  transition: background-color 0.12s ease-out, border-color 0.12s ease-out;
}

.dropdown-item:hover,
.dropdown-item.active {
  background: var(--color-surface-muted, #1c202a);
}

.dropdown-item.active {
  border-left-color: var(--color-accent, #4a8ad6);
}

.item-main {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  flex: 1;
  min-width: 0;
}

.item-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  width: 1.1rem;
  color: var(--color-text-muted, #8a8c92);
}

.dropdown-item.active .item-icon {
  color: var(--color-accent, #4a8ad6);
}

.glyph-icon {
  display: block;
}

.item-label {
  font-weight: 500;
  font-size: 0.82rem;
  color: var(--color-text, #e6e8ef);
  flex-shrink: 0;
  white-space: nowrap;
}

.item-detail {
  font-size: 0.75rem;
  color: var(--color-text-muted, #8a8c92);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex: 1;
  min-width: 0;
}

.item-badge {
  font-size: 0.68rem;
  padding: 0.05rem 0.3rem;
  border-radius: 2px;
  background: var(--color-border, #222631);
  color: var(--color-text-muted, #8a8c92);
  flex-shrink: 0;
  line-height: 1.2;
}

.dropdown-item.active .item-badge {
  background: var(--color-primary-soft, #152236);
  color: var(--color-accent, #4a8ad6);
}
</style>

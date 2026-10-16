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
            <template v-if="item.kind === 'command'">⚡</template>
            <template v-else-if="item.kind === 'file'">📄</template>
            <template v-else-if="item.kind === 'folder'">📁</template>
            <template v-else-if="item.kind === 'session'">💬</template>
            <template v-else>•</template>
          </span>
          <span class="item-label">{{ item.label }}</span>
          <span v-if="item.badge" class="item-badge">{{ item.badge }}</span>
        </div>
        <div v-if="item.detail" class="item-detail">{{ item.detail }}</div>
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
  max-height: 18rem;
  background: var(--color-surface, #ffffff);
  border: 1px solid var(--color-border-strong, #cbd5e1);
  border-radius: 10px;
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.16);
  overflow-y: auto;
  padding: 0.35rem;
}

.dropdown-loading {
  padding: 0.6rem 0.8rem;
  color: var(--color-text-muted, #64748b);
  font-size: 0.85rem;
  text-align: center;
}

.dropdown-list {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}

.dropdown-item {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  padding: 0.45rem 0.65rem;
  border-radius: 6px;
  cursor: pointer;
  transition: background-color 0.12s ease;
}

.dropdown-item:hover,
.dropdown-item.active {
  background: var(--color-surface-muted, #f1f5f9);
}

.item-main {
  display: flex;
  align-items: center;
  gap: 0.45rem;
}

.item-icon {
  font-size: 0.9rem;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.item-label {
  font-weight: 500;
  font-size: 0.88rem;
  color: var(--color-text, #0f172a);
}

.item-badge {
  font-size: 0.7rem;
  padding: 0.1rem 0.35rem;
  border-radius: 4px;
  background: var(--color-border, #e2e8f0);
  color: var(--color-text-muted, #64748b);
  margin-left: auto;
}

.dropdown-item.active .item-badge {
  background: var(--color-primary, #3b82f6);
  color: #ffffff;
}

.item-detail {
  font-size: 0.78rem;
  color: var(--color-text-muted, #64748b);
  padding-left: 1.35rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>

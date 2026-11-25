<script setup lang="ts">
// ListEmptyState: what a list shows when it has no rows, told as three states.
//
// An `el-table` with nothing in it says "No Data", and every page then wrote its
// own grey line under that. So the same product answered three completely
// different situations — data still in flight, data hidden behind a filter, and
// a genuinely empty library — with one identical silence, and the user cannot
// tell slow from empty from "your filter ate it". Here they diverge: a loading
// list gets a skeleton (a shape and a rhythm instead of a blank panel), a
// filtered-out list gets the way back out, and an empty library gets a visual
// anchor plus the place where the primary action lives (the default slot).
//
// The icon comes from the caller and must be the *same* glyph the page's nav
// entry uses (the organization centre: EditPen / ChatDotRound / User /
// Connection). An empty state echoes the navigation; it is not a random
// illustration. Vector only — no emoji.
import type { Component } from 'vue'

const props = withDefaults(
  defineProps<{
    /** loading: the first page is still in flight; filtered: rows exist but the filter
        emptied the table; empty: there is genuinely nothing yet. */
    state: 'loading' | 'filtered' | 'empty'
    /** The page's own nav glyph; unused while loading. */
    icon?: Component
    title: string
    hint?: string
    /** Label of the way-out button in the filtered state; '' hides it. */
    resetLabel?: string
    /** How many rows the skeleton stands in for. */
    rows?: number
  }>(),
  { resetLabel: '重置筛选', rows: 6 },
)

const emit = defineEmits<{ reset: [] }>()
</script>

<template>
  <div v-if="state === 'loading'" class="list-state list-state--loading" data-test="list-state-loading">
    <el-skeleton animated>
      <template #template>
        <div v-for="row in rows" :key="row" class="skeleton-row">
          <el-skeleton-item variant="text" class="skeleton-bar skeleton-bar-name" />
          <el-skeleton-item variant="text" class="skeleton-bar skeleton-bar-tag" />
          <el-skeleton-item variant="text" class="skeleton-bar skeleton-bar-body" />
          <el-skeleton-item variant="text" class="skeleton-bar skeleton-bar-meta" />
        </div>
      </template>
    </el-skeleton>
  </div>
  <div v-else class="list-state" :data-test="state === 'filtered' ? 'list-state-filtered' : 'list-state-empty'">
    <div v-if="icon" class="list-state-glyph">
      <el-icon :size="20"><component :is="icon" /></el-icon>
    </div>
    <p class="list-state-title">{{ title }}</p>
    <p v-if="hint" class="list-state-hint">{{ hint }}</p>
    <div v-if="state === 'filtered' && resetLabel" class="list-state-actions">
      <el-button size="small" @click="emit('reset')">{{ resetLabel }}</el-button>
    </div>
    <div v-else-if="state === 'empty'" class="list-state-actions">
      <slot />
    </div>
  </div>
</template>

<style scoped>
/* The table hands over the whole body height; this decides where the message
   sits in it — dead centre, because an empty state is reassurance, not a row. */
.list-state {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  height: 100%;
  min-height: 220px;
  padding: 28px 12px 32px;
  text-align: center;
}

.list-state--loading {
  /* A skeleton promises the shape of the rows to come, so it is shaped like
     this table: left aligned like the data, one bar per column group, and on
     the table's own row rhythm (34px) rather than the library's article
     paragraphs — the default `:rows` skeleton reads as “a page of text is
     loading”, which is not what is arriving. */
  align-items: stretch;
  justify-content: flex-start;
  width: 100%;
  min-height: 0;
  padding: 4px 12px 0;
  text-align: left;
}

.skeleton-row {
  display: flex;
  gap: 12px;
  align-items: center;
  height: 34px;
}

.skeleton-bar {
  height: 10px;
  margin: 0;
}

.skeleton-bar-name {
  width: 22%;
}

.skeleton-bar-tag {
  width: 52px;
  flex: none;
}

.skeleton-bar-body {
  flex: 1 1 auto;
}

.skeleton-bar-meta {
  width: 96px;
  flex: none;
}

/* The glyph sits in a hairline-outlined, silent square: it reads as a
   placeholder-for-the-object, not as floating decoration. */
.list-state-glyph {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  margin-bottom: 2px;
  color: var(--color-text-muted);
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
}

.list-state-title {
  margin: 0;
  font-size: var(--text-size-13);
  color: var(--color-text);
}

.list-state-hint {
  max-width: 46ch;
  margin: 0;
  font-size: var(--text-size-12);
  line-height: 1.55;
  color: var(--color-text-muted);
}

.list-state-actions {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-top: 6px;
}
</style>

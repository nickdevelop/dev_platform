<script setup lang="ts">
// ListToolbar: the one row that sits above every list page.
//
// The contract it enforces is the platform's list layout: global actions (create,
// import, …) on the left, the search form on the right, and the search form always
// ends with the same 搜索 / 重置 pair. Pages supply their buttons through the
// `actions` slot and their enum filters through the `filters` slot, so a filter
// row cannot drift into a different shape or a different button order per module.
//
// When each control takes effect is part of the contract, not an accident:
// enum filters (the `filters` slot) apply the moment they change, while the
// keyword field is a draft that only lands on 搜索 / Enter — a half-typed word
// should not flash the table. Clearing the field with its ✕ counts as a search
// with an empty keyword.
import { Search } from '@element-plus/icons-vue'

const props = withDefaults(
  defineProps<{
    /** The keyword being typed; the applied one lives in the caller. */
    keyword: string
    placeholder?: string
    searchTest?: string
    resetTest?: string
  }>(),
  {
    placeholder: '搜索…',
    searchTest: 'btn-list-search',
    resetTest: 'btn-list-reset',
  },
)

const emit = defineEmits<{
  'update:keyword': [value: string]
  search: []
  reset: []
}>()
</script>

<template>
  <div class="list-toolbar" data-test="list-toolbar">
    <div class="toolbar-actions">
      <slot name="actions" />
    </div>

    <!-- A real form so Enter in the field is the same code path as the button. -->
    <form class="toolbar-search" @submit.prevent="emit('search')">
      <slot name="filters" />

      <el-input
        size="small"
        class="toolbar-keyword"
        :prefix-icon="Search"
        :model-value="props.keyword"
        :placeholder="props.placeholder"
        clearable
        data-test="input-list-keyword"
        @update:model-value="emit('update:keyword', String($event))"
        @clear="emit('search')"
      />

      <el-button
        size="small"
        type="primary"
        native-type="submit"
        :data-test="props.searchTest"
      >
        搜索
      </el-button>
      <el-button
        size="small"
        :data-test="props.resetTest"
        @click="emit('reset')"
      >
        重置
      </el-button>
    </form>
  </div>
</template>

<style scoped>
/* Layout only, on the platform's 4px rhythm: the actions group hugs the left
   edge and the search form is pushed to the right edge of the same line. */
.list-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.toolbar-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.toolbar-search {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-left: auto;
}

.toolbar-keyword {
  width: 240px;
}

@media (max-width: 960px) {
  .toolbar-search {
    margin-left: 0;
    width: 100%;
  }

  .toolbar-keyword {
    flex: 1;
    width: auto;
    min-width: 160px;
  }
}
</style>

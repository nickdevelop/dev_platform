<script setup lang="ts">
withDefaults(
  defineProps<{
    vertical?: boolean
    dashed?: boolean
  }>(),
  {
    vertical: false,
    dashed: false,
  },
)
</script>

<template>
  <div
    v-if="vertical"
    class="inline-block h-full min-h-[1em] w-[1px] self-stretch bg-slate-200 dark:bg-slate-800 shrink-0"
    :class="[dashed ? 'border-r border-dashed border-slate-300 dark:border-slate-700 bg-transparent' : '']"
    role="separator"
    aria-orientation="vertical"
  />
  <div
    v-else
    class="flex items-center my-3 select-none"
    role="separator"
    aria-orientation="horizontal"
  >
    <div
      class="flex-1 h-[1px] bg-slate-200 dark:bg-slate-800"
      :class="[dashed ? 'border-t border-dashed border-slate-300 dark:border-slate-700 bg-transparent' : '']"
    />
    <span
      v-if="$slots.default"
      class="px-2.5 text-xs text-slate-400 dark:text-slate-500 font-medium shrink-0"
    >
      <slot />
    </span>
    <div
      v-if="$slots.default"
      class="flex-1 h-[1px] bg-slate-200 dark:bg-slate-800"
      :class="[dashed ? 'border-t border-dashed border-slate-300 dark:border-slate-700 bg-transparent' : '']"
    />
  </div>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    title?: string
    description?: string
    icon?: string
  }>(),
  {
    title: '',
    description: '',
    icon: '',
  },
)
</script>

<template>
  <div class="flex flex-col items-center justify-center p-8 text-center select-none">
    <div
      v-if="icon || $slots.icon"
      class="w-12 h-12 mb-3 rounded-full bg-slate-100 dark:bg-slate-800 flex items-center justify-center text-2xl text-slate-400 dark:text-slate-500"
    >
      <slot name="icon">{{ icon }}</slot>
    </div>

    <h3 v-if="title || $slots.title" class="text-base font-medium text-slate-800 dark:text-slate-200 mb-1">
      <slot name="title">{{ title }}</slot>
    </h3>

    <p
      v-if="description || $slots.description"
      class="text-xs text-slate-500 dark:text-slate-400 max-w-sm leading-relaxed mb-4"
    >
      <slot name="description">{{ description }}</slot>
    </p>

    <div v-if="$slots.actions" class="flex items-center gap-2">
      <slot name="actions" />
    </div>
  </div>
</template>

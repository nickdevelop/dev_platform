<script setup lang="ts">
withDefaults(
  defineProps<{
    size?: 'sm' | 'md' | 'lg'
    color?: string
  }>(),
  {
    size: 'md',
    color: 'currentColor',
  },
)
</script>

<template>
  <svg
    class="animate-spin inline-block shrink-0"
    :class="{
      'w-3.5 h-3.5': size === 'sm',
      'w-4 h-4': size === 'md',
      'w-5 h-5': size === 'lg',
    }"
    viewBox="0 0 24 24"
    fill="none"
    xmlns="http://www.w3.org/2000/svg"
    role="status"
    aria-label="加载中"
  >
    <circle
      class="opacity-25"
      cx="12"
      cy="12"
      r="10"
      stroke="currentColor"
      stroke-width="3"
    ></circle>
    <path
      class="opacity-75"
      fill="currentColor"
      d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
    ></path>
  </svg>
</template>

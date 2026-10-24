<script setup lang="ts">
withDefaults(
  defineProps<{
    ariaLabel?: string
  }>(),
  {
    ariaLabel: '树形列表',
  },
)
</script>

<template>
  <ul
    class="m-0 p-1 list-none overflow-y-auto space-y-0.5 outline-none"
    role="tree"
    :aria-label="ariaLabel"
  >
    <slot />
  </ul>
</template>

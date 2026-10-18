<script setup lang="ts">
// Settings center pane content: with the category links living in the shell's
// middle column (see ./SettingsNav.vue), this component only hosts the active
// section panels through the child router view.
import { RouterView } from 'vue-router'
</script>

<template>
  <div class="settings-view">
    <RouterView />
  </div>
</template>

<style scoped>
.settings-view {
  min-width: 0;
}
</style>

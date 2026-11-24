<script setup lang="ts">
// OrganizationView: Main container for Organization center.
// Hosts pure object governance for AI Assistants, AI Employees, and AI Teams.
import { RouterView } from 'vue-router'
</script>

<template>
  <main class="flex-1 min-w-0 h-full flex flex-col overflow-hidden bg-[var(--color-bg)]" data-test="org-workbench">
    <RouterView />
  </main>
</template>

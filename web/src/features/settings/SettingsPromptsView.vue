<script setup lang="ts">
// SettingsPromptsView: Re-export OrgPromptsView for backward compatibility.
// Prompt management is now located under Organization Center as 'AI 提示词'.
import OrgPromptsView from '../organization/prompts/OrgPromptsView.vue'
</script>

<template>
  <OrgPromptsView />
</template>

<script setup lang="ts">
// Write-only credential input. The field only ever holds characters the user
// just typed: a stored value is never fetched, rendered, or pre-filled. An
// empty value means "leave the stored credential unchanged".
const model = defineModel<string>({ default: '' })

defineProps<{
  // True when the resource already references a stored credential.
  hasStoredSecret?: boolean
}>()
</script>

<template>
  <div class="secret-field">
    <label for="mcp-secret">凭据值</label>
    <input
      id="mcp-secret"
      v-model="model"
      type="password"
      autocomplete="new-password"
      placeholder="输入新的凭据值（不会回显）"
      data-test="secret-value"
    />
    <p v-if="hasStoredSecret" class="hint" data-test="secret-stored">
      已保存凭据；留空则保持不变。
    </p>
  </div>
</template>

<style scoped>
.secret-field {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

input {
  padding: 0.35rem 0.5rem;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
}

.hint {
  color: #64748b;
  font-size: 0.8rem;
  margin: 0;
}
</style>

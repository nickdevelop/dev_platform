<script setup lang="ts">
import UiDialog from './UiDialog.vue'
import UiButton from '../atomic/UiButton.vue'
import UiAlert from '../atomic/UiAlert.vue'

const props = withDefaults(
  defineProps<{
    title: string
    message: string
    detail?: string
    confirmLabel?: string
    cancelLabel?: string
    variant?: 'primary' | 'danger'
    loading?: boolean
    error?: string
    widthClass?: string
  }>(),
  {
    detail: '',
    confirmLabel: '确定',
    cancelLabel: '取消',
    variant: 'danger',
    loading: false,
    error: '',
    widthClass: 'w-full max-w-md',
  },
)

const emit = defineEmits<{
  close: []
  confirm: []
}>()

function handleClose() {
  if (props.loading) return
  emit('close')
}

function handleConfirm() {
  if (props.loading) return
  emit('confirm')
}
</script>

<template>
  <UiDialog :title="title" :width-class="widthClass" @close="handleClose">
    <div class="space-y-3">
      <p class="text-sm font-medium text-platform-text leading-relaxed">
        {{ message }}
      </p>
      <p v-if="detail" class="text-xs text-platform-text-muted leading-relaxed">
        {{ detail }}
      </p>
      <slot />
      <UiAlert v-if="error" variant="error">
        {{ error }}
      </UiAlert>
    </div>

    <template #footer>
      <UiButton
        type="button"
        variant="secondary"
        :disabled="loading"
        data-test="confirm-cancel-btn"
        @click="handleClose"
      >
        {{ cancelLabel }}
      </UiButton>
      <UiButton
        type="button"
        :variant="variant"
        :loading="loading"
        :disabled="loading"
        data-test="confirm-accept-btn"
        @click="handleConfirm"
      >
        {{ confirmLabel }}
      </UiButton>
    </template>
  </UiDialog>
</template>

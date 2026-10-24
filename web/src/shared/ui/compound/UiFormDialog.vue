<script setup lang="ts">
import UiDialog from './UiDialog.vue'
import UiButton from '../atomic/UiButton.vue'
import UiAlert from '../atomic/UiAlert.vue'

const props = withDefaults(
  defineProps<{
    title: string
    confirmLabel?: string
    cancelLabel?: string
    confirmVariant?: 'primary' | 'danger'
    confirmDisabled?: boolean
    confirmLoading?: boolean
    error?: string
    widthClass?: string
  }>(),
  {
    confirmLabel: '确定',
    cancelLabel: '取消',
    confirmVariant: 'primary',
    confirmDisabled: false,
    confirmLoading: false,
    error: '',
    widthClass: 'w-full max-w-lg',
  },
)

const emit = defineEmits<{
  close: []
  submit: []
}>()

function handleSubmit() {
  if (props.confirmDisabled || props.confirmLoading) return
  emit('submit')
}
</script>

<template>
  <UiDialog :title="title" :width-class="widthClass" @close="emit('close')">
    <form class="space-y-4" @submit.prevent="handleSubmit">
      <UiAlert v-if="error" variant="error" class="mb-3">
        {{ error }}
      </UiAlert>

      <div class="space-y-3">
        <slot />
      </div>

      <div class="pt-4 flex items-center justify-end gap-2.5 border-t border-slate-100 dark:border-slate-800">
        <UiButton
          variant="secondary"
          type="button"
          data-test="dialog-cancel-btn"
          @click="emit('close')"
        >
          {{ cancelLabel }}
        </UiButton>
        <UiButton
          :variant="confirmVariant"
          type="submit"
          :disabled="confirmDisabled"
          :loading="confirmLoading"
          data-test="dialog-confirm-btn"
        >
          {{ confirmLabel }}
        </UiButton>
      </div>
    </form>
  </UiDialog>
</template>

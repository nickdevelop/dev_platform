<script setup lang="ts">
// EntityFormDialog: the shared shell for create / edit forms across the platform.
//
// Every "新增 / 编辑 X" form in the workbench is the same object: a dialog whose
// header carries a title, one line of purpose and the record id, and whose footer
// carries a shortcut hint plus a cancel / save pair. Element Plus owns the panel,
// the backdrop, the focus trap and the dismissal; this component owns the parts
// that must read identically in every module, so a migrated page and a not-yet
// migrated page cannot drift apart in chrome.
//
// Layout classes for slot content live in `shared/styles/entity-editor.css` —
// slot content compiles in the caller's scope, so a scoped block here would not
// reach it.
import { onMounted, onUnmounted, ref } from 'vue'

const props = withDefaults(
  defineProps<{
    title: string
    subtitle?: string
    /** Record id shown as a chip next to the title; omitted for creation. */
    identifier?: number | string | null
    /** 主动作按钮的文字。平台默认「确认」（保存并关闭）——标题栏已经写了新增 /
        编辑与记录号，按钮再复述一遍「创建助手 / 保存修改」是噪声。 */
    saveLabel?: string
    saving?: boolean
    width?: number | string
    /** `data-test` for the save button, so specs keep their selectors. */
    saveTest?: string
  }>(),
  {
    subtitle: '',
    saveLabel: '确认',
    identifier: null,
    saving: false,
    width: 880,
    saveTest: '',
  },
)

const emit = defineEmits<{
  save: []
  cancel: []
}>()

const visible = ref(true)

// Ctrl/Cmd+S is the platform's save shortcut. It lives here rather than in each
// form so that every editor answers the same keys.
function onKeyDown(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
    e.preventDefault()
    emit('save')
  }
}

onMounted(() => window.addEventListener('keydown', onKeyDown))
onUnmounted(() => window.removeEventListener('keydown', onKeyDown))

function close() {
  visible.value = false
}

defineExpose({ close })
</script>

<template>
  <el-dialog
    v-model="visible"
    class="entity-dialog"
    :width="width"
    :close-on-click-modal="false"
    v-bind="$attrs"
    @closed="emit('cancel')"
  >
    <template #header>
      <div class="entity-dialog-head">
        <div class="min-w-0">
          <div class="flex items-center gap-2">
            <h2 class="entity-dialog-title">{{ title }}</h2>
            <span v-if="identifier !== null && identifier !== ''" class="entity-id-chip font-mono">
              #{{ identifier }}
            </span>
          </div>
          <p v-if="subtitle" class="entity-dialog-subtitle">{{ subtitle }}</p>
        </div>
        <slot name="head-extra" />
      </div>
    </template>

    <slot />

    <template #footer>
      <div class="entity-dialog-foot">
        <span class="entity-shortcut-hint">Ctrl+S 保存 · Esc 关闭</span>
        <div class="entity-dialog-actions">
          <!-- 全平台同一对动作：取消 = 关闭不保存，确认 = 保存数据。不带图标——
               两个字的按钮配一枚图标在这对动作上没有信息量，间距也由库负责。 -->
          <el-button @click="close">取消</el-button>
          <el-button type="primary" :loading="saving" :data-test="saveTest" @click="emit('save')">
            {{ saveLabel }}
          </el-button>
        </div>
      </div>
    </template>
  </el-dialog>
</template>

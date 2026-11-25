<script setup lang="ts">
// PromptManageDialog: create or edit one prompt template.
//
// Element Plus carries the form: `el-dialog` for the panel, `el-form` with rules
// for the two required fields (inline, next to the field that failed, instead of
// an error banner above the whole form), and `ElMessage` for the result. The
// form element keeps its `prompt-dialog-form` class and submit handler so the
// panel can still be driven by a plain submit.
import { computed, reactive, ref } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { describeApiError } from '../../api/client'
import type { Prompt } from '../../api/prompts'
import { usePromptsStore } from '../../stores/prompts'

const props = defineProps<{
  prompt?: Prompt | null
  defaultType?: 'role' | 'flow'
}>()

const emit = defineEmits<{
  close: []
  saved: [prompt: Prompt]
}>()

const promptsStore = usePromptsStore()

const visible = ref(true)
const formRef = ref<FormInstance>()
const saving = ref(false)

const form = reactive({
  name: props.prompt?.name ?? '',
  type: (props.prompt?.type ?? props.defaultType ?? 'role') as 'role' | 'flow',
  description: props.prompt?.description ?? '',
  content: props.prompt?.content ?? '',
})

const rules: FormRules<typeof form> = {
  name: [{ required: true, message: '请输入提示词名称', trigger: 'blur' }],
  content: [{ required: true, message: '请输入提示词内容', trigger: 'blur' }],
}

const isEditing = computed(() => !!props.prompt)

const typeDescription = computed(() =>
  form.type === 'flow'
    ? '流程类偏向工作流程与协同任务规范，供多 Agent 协同团队统一引用。'
    : '角色类设定 Agent 人设、专业背景与思考准则，可在助手与数字员工中统一引用。',
)

function onClose() {
  visible.value = false
}

async function submit() {
  if (saving.value) return
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  saving.value = true
  const payload = {
    name: form.name.trim(),
    type: form.type,
    description: form.description.trim(),
    content: form.content,
  }

  try {
    const saved: Prompt | undefined =
      isEditing.value && props.prompt
        ? await promptsStore.updatePrompt(props.prompt.id, payload)
        : await promptsStore.createPrompt(payload)
    // The store returns undefined when the session expired mid-save; the auth
    // store is already showing the re-auth flow, so stay quiet here.
    if (saved) {
      ElMessage.success(isEditing.value ? `已保存「${saved.name}」` : `已创建 AI 提示词「${saved.name}」`)
      emit('saved', saved)
      emit('close')
    }
  } catch (err) {
    ElMessage.error(describeApiError(err))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="isEditing ? '编辑 AI 提示词' : '新建 AI 提示词'"
    width="640"
    :close-on-click-modal="false"
    :data-test="`prompt-dialog-${isEditing ? 'edit' : 'create'}`"
    @closed="emit('close')"
  >
    <el-form
      ref="formRef"
      class="prompt-dialog-form"
      :model="form"
      :rules="rules"
      label-position="top"
      @submit.prevent="submit"
    >
      <el-form-item label="提示词名称" prop="name">
        <el-input
          v-model="form.name"
          placeholder="例如：代码审查助手、Git 提交信息生成"
          data-test="prompt-name-input"
        />
      </el-form-item>

      <el-form-item label="提示词类型" prop="type">
        <div class="type-block">
          <el-radio-group v-model="form.type">
            <el-radio-button value="role" border>角色类 (Role)</el-radio-button>
            <el-radio-button value="flow" border>流程类 (Flow)</el-radio-button>
          </el-radio-group>
          <p class="type-desc">{{ typeDescription }}</p>
        </div>
      </el-form-item>

      <el-form-item label="描述 / 用途说明" prop="description">
        <el-input
          v-model="form.description"
          placeholder="简要说明该提示词的应用场景与期望产出"
          data-test="prompt-desc-input"
        />
      </el-form-item>

      <el-form-item label="提示词内容" prop="content">
        <el-input
          v-model="form.content"
          type="textarea"
          :rows="10"
          resize="vertical"
          placeholder="输入提示词内容，例如：&#10;请对以下代码进行审查，并指出潜在缺陷与优化建议。"
          class="content-input"
          data-test="prompt-content-input"
        />
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="entity-dialog-actions justify-end">
        <el-button @click="onClose">取消</el-button>
        <el-button type="primary" :loading="saving" data-test="prompt-save-btn" @click="submit">
          确认
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<style scoped>
/* The library's own label markup is not reached into from here (spec §六): the
   type choice is a plain bordered radio pair, and the consequence of the current
   choice is a caption we own. */
.type-block {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.type-desc {
  margin: 0;
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
  line-height: 1.5;
}

/* The body is code-shaped text. `el-input`'s textarea inherits its face and
   size, so styling the control root is enough. */
.content-input {
  font-family: var(--font-mono);
  font-size: var(--text-size-12);
  line-height: 1.6;
}
</style>

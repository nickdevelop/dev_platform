<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import EntityFormDialog from '../../shared/components/EntityFormDialog.vue'
import type {
  BotChannel,
  ChannelDescriptor,
  CreateBotChannelInput,
  UpdateBotChannelInput,
} from '../../api/bot'
import { useModelsStore } from '../../stores/models'

const props = defineProps<{
  channel?: BotChannel | null
  saving?: boolean
  descriptors: ChannelDescriptor[]
}>()

const emit = defineEmits<{
  save: [payload: CreateBotChannelInput | UpdateBotChannelInput]
  cancel: []
}>()

const modelsStore = useModelsStore()

const isEdit = computed(() => Boolean(props.channel))
const formRef = ref<FormInstance>()

const form = reactive({
  name: '',
  channelType: 'wechat',
  modelId: 0,
  autoNotify: true,
})

const rules: FormRules<typeof form> = {
  name: [
    { required: true, message: '请输入渠道名称', trigger: 'blur' },
    { min: 1, max: 60, message: '名称长度需在 1 到 60 个字符之间', trigger: 'blur' },
  ],
  channelType: [{ required: true, message: '请选择渠道协议', trigger: 'change' }],
}

const usableTextModels = computed(() => {
  const list: Array<{ id: number; label: string; serviceName: string }> = []
  for (const svc of modelsStore.services) {
    for (const m of svc.models ?? []) {
      if (m.usable && m.supportsText) {
        list.push({
          id: m.id,
          label: m.name || m.modelId,
          serviceName: svc.name,
        })
      }
    }
  }
  return list
})

function syncFromProps() {
  if (props.channel) {
    form.name = props.channel.name
    form.channelType = props.channel.channelType || 'wechat'
    form.modelId = props.channel.modelId || 0
    form.autoNotify = props.channel.autoNotify
  } else {
    form.name = '微信 Bot'
    form.channelType = props.descriptors[0]?.type || 'wechat'
    form.modelId = usableTextModels.value[0]?.id || 0
    form.autoNotify = true
  }
}

watch(() => props.channel, syncFromProps, { immediate: true })

watch(
  usableTextModels,
  (list) => {
    if (!isEdit.value && form.modelId === 0 && list.length > 0) {
      form.modelId = list[0].id
    }
  },
  { immediate: true },
)

onMounted(() => {
  if (modelsStore.services.length === 0) void modelsStore.load()
})

async function submit() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  if (isEdit.value) {
    const payload: UpdateBotChannelInput = {
      name: form.name.trim(),
      modelId: form.modelId || 0,
      autoNotify: form.autoNotify,
    }
    emit('save', payload)
  } else {
    const payload: CreateBotChannelInput = {
      name: form.name.trim(),
      channelType: form.channelType,
      modelId: form.modelId || 0,
      autoNotify: form.autoNotify,
    }
    emit('save', payload)
  }
}
</script>

<template>
  <EntityFormDialog
    :title="isEdit ? `编辑 Bot 渠道 · ${channel?.name}` : '新建 Bot 渠道'"
    :subtitle="
      isEdit
        ? '调整渠道名称、专属对话使用的模型与异步推送开关。'
        : '创建渠道将同时完成两件事：建立外部渠道连接（微信扫码）+ 基于 Bot 引擎创建渠道专属对话。'
    "
    :identifier="isEdit ? channel?.id : null"
    :saving="saving"
    :save-label="isEdit ? '保存' : '创建并扫码连接'"
    :width="560"
    save-test="save-bot-channel"
    data-test="bot-channel-dialog"
    @save="submit"
    @cancel="emit('cancel')"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
      <section v-if="!isEdit" class="entity-form-section">
        <div class="bot-steps-banner">
          <div class="bot-step-item">
            <span class="entity-section-num">01</span>
            <span>创建后立即弹出扫码窗口，建立微信连接</span>
          </div>
          <div class="bot-step-item">
            <span class="entity-section-num">02</span>
            <span>基于 Bot 引擎自动生成该渠道专属对话</span>
          </div>
        </div>
      </section>

      <section class="entity-form-section">
        <div class="entity-grid-2">
          <el-form-item label="渠道名称" prop="name">
            <el-input
              v-model="form.name"
              placeholder="例如：个人微信 Bot"
              data-test="bot-channel-name-input"
            />
          </el-form-item>

          <el-form-item label="接入渠道" prop="channelType">
            <el-select
              v-model="form.channelType"
              :disabled="isEdit"
              placeholder="选择接入渠道"
              class="w-full"
            >
              <el-option
                v-for="desc in descriptors"
                :key="desc.type"
                :label="desc.label"
                :value="desc.type"
              />
            </el-select>
          </el-form-item>
        </div>

        <el-form-item label="专属对话推理模型">
          <el-select
            v-model="form.modelId"
            clearable
            filterable
            placeholder="选择渠道专属对话使用的模型"
            class="w-full"
          >
            <el-option
              v-for="m in usableTextModels"
              :key="m.id"
              :label="m.label"
              :value="m.id"
            >
              <span>{{ m.label }}</span>
              <span class="entity-option-sub">{{ m.serviceName }}</span>
            </el-option>
          </el-select>
          <p class="entity-field-hint">
            该对话基于平台 Bot ReAct 引擎运行，按需动态查询工程与 Agent 后端，无需预绑定工程或人设。
          </p>
        </el-form-item>
      </section>

      <section class="entity-form-section">
        <div class="entity-switch-row">
          <div>
            <div class="entity-switch-title">会话执行结果主动回传</div>
            <div class="entity-switch-desc">
              由此渠道创建的平台会话在完成或异常时，自动将结果摘要推送回微信并记录到专属对话中。
            </div>
          </div>
          <el-switch v-model="form.autoNotify" />
        </div>
      </section>
    </el-form>
  </EntityFormDialog>
</template>

<style scoped>
.bot-steps-banner {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface-muted);
}

.bot-step-item {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: var(--text-size-12);
  color: var(--color-text);
}
</style>

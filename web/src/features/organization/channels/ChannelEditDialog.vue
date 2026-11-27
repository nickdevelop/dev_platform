<script setup lang="ts">
// ChannelEditDialog: create or edit a message channel inside EntityFormDialog.
//
// Supports single-channel context binding to either:
//   - `assistant`: an AI Assistant (chat group), with optional model override
//   - `agent`:     a Project (+ optional Git worktree) + Role prompt + Builtin Agent
import { computed, onMounted, reactive, ref, watch } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { listAgentBackendModels, type ModelInfo } from '../../../api/agents'
import type {
  BotChannel,
  BotTargetMode,
  ChannelDescriptor,
  CreateBotChannelInput,
  UpdateBotChannelInput,
} from '../../../api/bot'
import EntityFormDialog from '../../../shared/components/EntityFormDialog.vue'
import { useAgentsStore } from '../../../stores/agents'
import { useChatsStore } from '../../../stores/chats'
import { useModelsStore } from '../../../stores/models'
import { useProjectsStore } from '../../../stores/projects'
import { usePromptsStore } from '../../../stores/prompts'
import { useWorktreesStore } from '../../../stores/worktrees'

const props = defineProps<{
  channel?: BotChannel | null
  saving?: boolean
  descriptors: ChannelDescriptor[]
}>()

const emit = defineEmits<{
  save: [payload: CreateBotChannelInput | UpdateBotChannelInput]
  cancel: []
}>()

const chatsStore = useChatsStore()
const modelsStore = useModelsStore()
const projectsStore = useProjectsStore()
const promptsStore = usePromptsStore()
const worktreesStore = useWorktreesStore()
const agentsStore = useAgentsStore()

const isEdit = computed(() => Boolean(props.channel))
const formRef = ref<FormInstance>()

const agentModels = ref<ModelInfo[]>([])
const loadingAgentModels = ref(false)

const form = reactive({
  name: '',
  channelType: 'wechat',
  targetMode: 'assistant' as BotTargetMode,
  chatGroupId: 0,
  modelId: 0,
  defaultProjectId: 0,
  worktreeId: 0,
  promptId: 0,
  systemPrompt: '',
  agentModel: '',
  typingIndicator: true,
  autoNotify: true,
  maxReplyLength: 2000,
})

const targetModeOptions = [
  { label: 'AI 助手', value: 'assistant' },
  { label: '内置 Agent', value: 'agent' },
]

const rules = computed<FormRules<typeof form>>(() => ({
  name: [
    { required: true, message: '请输入渠道名称', trigger: 'blur' },
    { min: 1, max: 60, message: '名称长度需在 1 到 60 个字符之间', trigger: 'blur' },
  ],
  channelType: [{ required: true, message: '请选择接入协议', trigger: 'change' }],
  chatGroupId: [
    {
      validator: (_rule, _value, callback) => {
        if (form.targetMode === 'assistant' && (!form.chatGroupId || form.chatGroupId <= 0)) {
          callback(new Error('请选择要绑定的 AI 助手'))
          return
        }
        callback()
      },
      trigger: 'change',
    },
  ],
  defaultProjectId: [
    {
      validator: (_rule, _value, callback) => {
        if (form.targetMode === 'agent' && (!form.defaultProjectId || form.defaultProjectId <= 0)) {
          callback(new Error('请选择目标项目'))
          return
        }
        callback()
      },
      trigger: 'change',
    },
  ],
}))

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

const rolePrompts = computed(() =>
  (promptsStore.prompts ?? []).filter((p) => p.type === 'role'),
)

const selectedProject = computed(() =>
  form.defaultProjectId ? projectsStore.byId(form.defaultProjectId) : undefined,
)

const availableWorktrees = computed(() =>
  form.defaultProjectId ? worktreesStore.availableForProject(form.defaultProjectId) : [],
)

const selectedPromptPreview = computed(() => {
  if (!form.promptId) return ''
  return promptsStore.byId(form.promptId)?.content || ''
})

// Resolve the embedded agent descriptor from the server-supplied catalog when
// creating a new channel, or preserve the backend id stored on the channel.
const agentBackendId = computed(() => {
  if (props.channel?.defaultBackend) return props.channel.defaultBackend
  const embedded = agentsStore.backends.find(
    (b) => !b.serviceSupported && b.capabilities?.toolConfirmation,
  )
  return embedded?.backend || agentsStore.orderedBackends[0]?.backend || ''
})

const agentBackendLabel = computed(() => {
  if (agentBackendId.value) {
    const label = agentsStore.backendLabel(agentBackendId.value)
    if (label && label !== agentBackendId.value) return label
  }
  return '内置 Agent'
})

async function loadAgentModels(backendId: string) {
  if (!backendId) {
    agentModels.value = []
    return
  }
  loadingAgentModels.value = true
  try {
    const res = await listAgentBackendModels(backendId)
    agentModels.value = (res.models ?? []).filter((m) => m.enabled !== false)
  } catch {
    agentModels.value = []
  } finally {
    loadingAgentModels.value = false
  }
}

function syncFromProps() {
  formRef.value?.clearValidate()
  if (props.channel) {
    form.name = props.channel.name
    form.channelType = props.channel.channelType || 'wechat'
    form.targetMode = props.channel.targetMode === 'agent' ? 'agent' : 'assistant'
    form.chatGroupId = props.channel.chatGroupId || 0
    form.modelId = props.channel.modelId || 0
    form.defaultProjectId = props.channel.defaultProjectId || 0
    form.worktreeId = props.channel.worktreeId || 0
    form.promptId = props.channel.promptId || 0
    form.systemPrompt = props.channel.systemPrompt || ''
    form.agentModel = props.channel.agentModel || ''
    form.typingIndicator = props.channel.typingIndicator ?? true
    form.autoNotify = props.channel.autoNotify ?? true
    form.maxReplyLength = props.channel.maxReplyLength || 2000
  } else {
    form.name = '微信消息渠道'
    form.channelType = props.descriptors[0]?.type || 'wechat'
    form.targetMode = 'assistant'
    form.chatGroupId = chatsStore.groups[0]?.id || 0
    form.modelId = 0
    form.defaultProjectId = projectsStore.orderedProjects[0]?.id || 0
    form.worktreeId = 0
    form.promptId = rolePrompts.value[0]?.id || 0
    form.systemPrompt = ''
    form.agentModel = ''
    form.typingIndicator = true
    form.autoNotify = true
    form.maxReplyLength = 2000
  }
}

watch(() => props.channel, syncFromProps, { immediate: true })

watch(
  () => form.defaultProjectId,
  (projectId) => {
    if (!projectId) {
      form.worktreeId = 0
      return
    }
    const proj = projectsStore.byId(projectId)
    if (proj?.isGit) {
      void worktreesStore.load(projectId)
    } else {
      form.worktreeId = 0
    }
  },
  { immediate: true },
)

watch(
  agentBackendId,
  (id) => {
    if (id) void loadAgentModels(id)
  },
  { immediate: true },
)

onMounted(async () => {
  await Promise.all([
    modelsStore.services.length === 0 ? modelsStore.load() : Promise.resolve(),
    chatsStore.groups.length === 0 ? chatsStore.loadGroups() : Promise.resolve(),
    projectsStore.projects.length === 0 ? projectsStore.load() : Promise.resolve(),
    promptsStore.prompts.length === 0 ? promptsStore.loadPrompts() : Promise.resolve(),
    agentsStore.backends.length === 0 ? agentsStore.loadBackends() : Promise.resolve(),
  ])
  if (!isEdit.value) {
    if (!form.chatGroupId && chatsStore.groups.length > 0) {
      form.chatGroupId = chatsStore.groups[0].id
    }
    if (!form.defaultProjectId && projectsStore.orderedProjects.length > 0) {
      form.defaultProjectId = projectsStore.orderedProjects[0].id
    }
  }
})

async function submit() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  if (isEdit.value) {
    const payload: UpdateBotChannelInput = {
      name: form.name.trim(),
      targetMode: form.targetMode,
      chatGroupId: form.targetMode === 'assistant' ? form.chatGroupId : 0,
      modelId: form.targetMode === 'assistant' ? form.modelId || 0 : 0,
      defaultProjectId: form.targetMode === 'agent' ? form.defaultProjectId : 0,
      worktreeId: form.targetMode === 'agent' ? form.worktreeId || 0 : 0,
      promptId: form.targetMode === 'agent' ? form.promptId || 0 : 0,
      systemPrompt: form.targetMode === 'agent' ? form.systemPrompt.trim() : '',
      agentModel: form.targetMode === 'agent' ? form.agentModel.trim() : '',
      typingIndicator: form.typingIndicator,
      autoNotify: form.autoNotify,
      maxReplyLength: form.maxReplyLength,
    }
    emit('save', payload)
  } else {
    const payload: CreateBotChannelInput = {
      name: form.name.trim(),
      channelType: form.channelType,
      targetMode: form.targetMode,
      chatGroupId: form.targetMode === 'assistant' ? form.chatGroupId : undefined,
      modelId: form.targetMode === 'assistant' && form.modelId ? form.modelId : undefined,
      defaultProjectId: form.targetMode === 'agent' ? form.defaultProjectId : undefined,
      worktreeId: form.targetMode === 'agent' && form.worktreeId ? form.worktreeId : undefined,
      promptId: form.targetMode === 'agent' && form.promptId ? form.promptId : undefined,
      systemPrompt:
        form.targetMode === 'agent' && form.systemPrompt.trim()
          ? form.systemPrompt.trim()
          : undefined,
      agentModel:
        form.targetMode === 'agent' && form.agentModel.trim()
          ? form.agentModel.trim()
          : undefined,
      typingIndicator: form.typingIndicator,
      autoNotify: form.autoNotify,
      maxReplyLength: form.maxReplyLength,
    }
    emit('save', payload)
  }
}
</script>

<template>
  <EntityFormDialog
    :title="isEdit ? `编辑消息渠道 · ${channel?.name}` : '新建消息渠道'"
    :subtitle="
      isEdit
        ? '调整渠道绑定服务、单渠道上下文关联及投递策略；保存后若切换模式或核心绑定会重置当前关联上下文。'
        : '配置接入渠道与单渠道绑定服务（AI 助手 或 项目 + 角色 + 内置 Agent），首条消息到达时自动建链。'
    "
    :identifier="isEdit ? channel?.id : null"
    :saving="saving"
    :width="720"
    save-test="save-channel"
    data-test="channel-edit-dialog"
    @save="submit"
    @cancel="emit('cancel')"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
      <section class="entity-form-section">
        <header class="entity-section-head entity-section-head-split">
          <div>
            <h3 class="entity-section-title">
              <span class="entity-section-num font-mono">01</span>渠道信息与绑定模式
            </h3>
            <p class="entity-section-desc">
              单渠道绑定单一 AI 服务形态，外部对话与内部历史隔离且支持发送 /new 开启新上下文。
            </p>
          </div>
          <el-segmented
            v-model="form.targetMode"
            :options="targetModeOptions"
            size="small"
            data-test="segment-target-mode"
          />
        </header>

        <div class="entity-grid-2">
          <el-form-item label="渠道名称" prop="name">
            <el-input
              v-model="form.name"
              placeholder="例如：微信研发助理"
              data-test="channel-name-input"
            />
          </el-form-item>

          <el-form-item label="接入协议" prop="channelType">
            <el-select
              v-model="form.channelType"
              :disabled="isEdit"
              placeholder="选择接入协议"
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
      </section>

      <section v-if="form.targetMode === 'assistant'" class="entity-form-section">
        <header class="entity-section-head">
          <h3 class="entity-section-title">
            <span class="entity-section-num font-mono">02</span>AI 助手绑定配置
          </h3>
          <p class="entity-section-desc">
            收到消息时自动创建标记为 BOT 的专属对话；该对话不会混入桌面快捷助手历史列表。
          </p>
        </header>

        <div class="entity-grid-2">
          <el-form-item label="绑定 AI 助手" prop="chatGroupId">
            <el-select
              v-model="form.chatGroupId"
              filterable
              placeholder="选择组织中心 AI 助手"
              class="w-full"
              data-test="select-channel-assistant"
            >
              <el-option
                v-for="g in chatsStore.groups"
                :key="g.id"
                :label="g.name"
                :value="g.id"
              >
                <span>{{ g.name }}</span>
                <span class="entity-option-sub">
                  {{ g.enablePlatformTools ? '平台工具已启用' : '基础对话' }}
                </span>
              </el-option>
            </el-select>
          </el-form-item>

          <el-form-item label="渠道模型覆盖（可选）">
            <el-select
              v-model="form.modelId"
              clearable
              filterable
              placeholder="默认跟随所选 AI 助手模型"
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
          </el-form-item>
        </div>
      </section>

      <section v-else class="entity-form-section">
        <header class="entity-section-head">
          <h3 class="entity-section-title">
            <span class="entity-section-num font-mono">02</span>项目 · 角色 · 内置 Agent 绑定
          </h3>
          <p class="entity-section-desc">
            基于 {{ agentBackendLabel }} 在指定项目下启动编码会话，默认启用自动放行（permissionMode = auto）。
          </p>
        </header>

        <div class="entity-grid-2">
          <el-form-item label="目标项目" prop="defaultProjectId">
            <el-select
              v-model="form.defaultProjectId"
              filterable
              placeholder="选择要服务的项目"
              class="w-full"
              data-test="select-channel-project"
            >
              <el-option
                v-for="proj in projectsStore.orderedProjects"
                :key="proj.id"
                :label="proj.name"
                :value="proj.id"
              >
                <span>{{ proj.name }}</span>
                <span class="entity-option-sub">{{ proj.path }}</span>
              </el-option>
            </el-select>
          </el-form-item>

          <el-form-item label="工作目录 / Worktree">
            <el-select
              v-model="form.worktreeId"
              placeholder="项目主目录"
              class="w-full"
              :disabled="!selectedProject?.isGit || availableWorktrees.length === 0"
            >
              <el-option label="项目主目录 (Default)" :value="0" />
              <el-option
                v-for="wt in availableWorktrees"
                :key="wt.id"
                :label="wt.branch || wt.path"
                :value="wt.id"
              />
            </el-select>
          </el-form-item>

          <el-form-item label="绑定角色提示词（可选）">
            <el-select
              v-model="form.promptId"
              clearable
              filterable
              placeholder="选择角色类提示词"
              class="w-full"
              data-test="select-channel-prompt"
            >
              <el-option label="不使用预置角色提示词" :value="0" />
              <el-option
                v-for="p in rolePrompts"
                :key="p.id"
                :label="p.name"
                :value="p.id"
              >
                <span>{{ p.name }}</span>
                <span v-if="p.description" class="entity-option-sub">{{ p.description }}</span>
              </el-option>
            </el-select>
          </el-form-item>

          <el-form-item label="Agent 运行模型（可选）">
            <el-select
              v-model="form.agentModel"
              clearable
              filterable
              allow-create
              default-first-option
              :loading="loadingAgentModels"
              placeholder="留空则跟随系统默认模型"
              class="w-full"
            >
              <el-option
                v-for="m in agentModels"
                :key="m.id"
                :label="m.name || m.id"
                :value="m.id"
              >
                <span>{{ m.name || m.id }}</span>
                <span class="entity-option-sub">{{ m.provider }}</span>
              </el-option>
            </el-select>
          </el-form-item>
        </div>

        <div v-if="selectedPromptPreview" class="entity-prompt-preview font-mono select-text mb-3">
          {{ selectedPromptPreview }}
        </div>

        <el-form-item label="附加角色指令 / 自定义人设（可选）">
          <el-input
            v-model="form.systemPrompt"
            type="textarea"
            :rows="3"
            resize="vertical"
            class="entity-mono-input"
            placeholder="仅在每次新建渠道 Agent 会话的首条消息注入（与上方角色提示词二选一或补充）"
          />
        </el-form-item>
      </section>

      <section class="entity-form-section">
        <header class="entity-section-head">
          <h3 class="entity-section-title">
            <span class="entity-section-num font-mono">03</span>消息投递与通知策略
          </h3>
          <p class="entity-section-desc">
            每个消息渠道独立配置「正在输入」心跳、异步通知以及长回复分片长度上限。
          </p>
        </header>

        <div class="space-y-2">
          <div class="entity-switch-row">
            <div>
              <div class="entity-switch-title">发送「正在输入」状态提示</div>
              <div class="entity-switch-desc">
                推理或工具执行期间向外部 IM 客户端推送 Typing 状态指示。
              </div>
            </div>
            <el-switch v-model="form.typingIndicator" />
          </div>

          <div class="entity-switch-row">
            <div>
              <div class="entity-switch-title">任务结果主动回传</div>
              <div class="entity-switch-desc">
                关联会话或后台任务完成、异常时自动将摘要推送回来源渠道用户。
              </div>
            </div>
            <el-switch v-model="form.autoNotify" />
          </div>

          <div class="entity-switch-row">
            <div>
              <div class="entity-switch-title">单条回复分片上限（字符）</div>
              <div class="entity-switch-desc">
                超长文本回复按段落边界自动切分发送，适配 IM 消息体积限制。
              </div>
            </div>
            <el-input-number
              v-model="form.maxReplyLength"
              :min="500"
              :max="4000"
              :step="200"
            />
          </div>
        </div>
      </section>
    </el-form>
  </EntityFormDialog>
</template>

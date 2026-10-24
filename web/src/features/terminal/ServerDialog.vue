<script setup lang="ts">
import { computed, ref } from 'vue'
import { testServerConnection, type TerminalServer } from '../../api/terminal'
import { useTerminalStore } from '../../stores/terminal'
import { useAuthStore } from '../../stores/auth'
import { UiDialog, UiButton, UiInput, UiTextarea, UiAlert } from '../../shared/ui'

const props = defineProps<{
  server?: TerminalServer | null
}>()

const emit = defineEmits<{
  close: []
  saved: [server: TerminalServer]
}>()

const terminalStore = useTerminalStore()
const authStore = useAuthStore()

const isEditing = computed(() => !!props.server && props.server.id > 0)
const title = computed(() => (isEditing.value ? '编辑服务器' : '添加第三方服务器'))

const name = ref(props.server?.name ?? '')
const host = ref(props.server?.host ?? '')
const port = ref(props.server?.port ?? 22)
const username = ref(props.server?.username ?? 'root')
const authType = ref<'password' | 'key'>(
  props.server?.auth_type === 'key' ? 'key' : 'password',
)
const secret = ref('')
const showPassword = ref(false)

const testing = ref(false)
const testResult = ref<{ ok: boolean; message: string } | null>(null)
const saving = ref(false)
const errorMessage = ref('')

async function onTest() {
  if (!host.value.trim()) {
    errorMessage.value = '请输入主机地址'
    return
  }
  if (!username.value.trim()) {
    errorMessage.value = '请输入用户名'
    return
  }
  if (!isEditing.value && !secret.value) {
    errorMessage.value = '请输入认证密码或私钥'
    return
  }

  testing.value = true
  testResult.value = null
  errorMessage.value = ''

  try {
    const res = await testServerConnection({
      host: host.value.trim(),
      port: Number(port.value) || 22,
      username: username.value.trim(),
      auth_type: authType.value,
      secret: secret.value || undefined,
      server_id: isEditing.value && !secret.value ? props.server?.id : undefined,
    })
    testResult.value = {
      ok: true,
      message: res.message || '连接成功',
    }
  } catch (err: unknown) {
    testResult.value = {
      ok: false,
      message: err instanceof Error ? err.message : '连接测试失败',
    }
  } finally {
    testing.value = false
  }
}

async function onSubmit() {
  if (!name.value.trim()) {
    errorMessage.value = '请输入服务器名称'
    return
  }
  if (!host.value.trim()) {
    errorMessage.value = '请输入主机地址'
    return
  }
  if (!username.value.trim()) {
    errorMessage.value = '请输入用户名'
    return
  }
  if (!isEditing.value && !secret.value) {
    errorMessage.value = authType.value === 'password' ? '请输入密码' : '请输入私钥文本'
    return
  }

  saving.value = true
  errorMessage.value = ''

  try {
    let saved: TerminalServer
    if (isEditing.value && props.server) {
      saved = await terminalStore.updateServer(props.server.id, {
        name: name.value.trim(),
        host: host.value.trim(),
        port: Number(port.value) || 22,
        username: username.value.trim(),
        auth_type: authType.value,
        secret: secret.value ? secret.value : undefined,
      })
    } else {
      saved = await terminalStore.createServer({
        name: name.value.trim(),
        host: host.value.trim(),
        port: Number(port.value) || 22,
        username: username.value.trim(),
        auth_type: authType.value,
        secret: secret.value,
      })
    }
    emit('saved', saved)
    emit('close')
  } catch (err: unknown) {
    errorMessage.value = err instanceof Error ? err.message : '保存失败'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <UiDialog :title="title" width-class="w-full max-w-lg" @close="emit('close')">
    <form class="server-form space-y-4" @submit.prevent="onSubmit">
      <UiAlert v-if="errorMessage" variant="error" class="form-error">
        {{ errorMessage }}
      </UiAlert>

      <div class="form-row flex flex-col gap-1.5">
        <label class="form-label text-xs font-semibold text-slate-700 dark:text-slate-300" for="server-name">
          服务器名称 <span class="text-red-500">*</span>
        </label>
        <UiInput
          id="server-name"
          v-model="name"
          placeholder="例如：开发环境、数据库服务器"
          autofocus
          required
          block
        />
      </div>

      <div class="form-grid grid grid-cols-3 gap-3">
        <div class="form-row col-span-2 flex flex-col gap-1.5">
          <label class="form-label text-xs font-semibold text-slate-700 dark:text-slate-300" for="server-host">
            主机地址 (Host) <span class="text-red-500">*</span>
          </label>
          <UiInput
            id="server-host"
            v-model="host"
            placeholder="IP 或域名"
            required
            block
          />
        </div>

        <div class="form-row port-col flex flex-col gap-1.5">
          <label class="form-label text-xs font-semibold text-slate-700 dark:text-slate-300" for="server-port">端口</label>
          <UiInput
            id="server-port"
            v-model.number="port"
            type="number"
            min="1"
            max="65535"
            placeholder="22"
            block
          />
        </div>
      </div>

      <div class="form-row flex flex-col gap-1.5">
        <label class="form-label text-xs font-semibold text-slate-700 dark:text-slate-300" for="server-username">
          用户名 (Username) <span class="text-red-500">*</span>
        </label>
        <UiInput
          id="server-username"
          v-model="username"
          placeholder="例如：root、ubuntu"
          required
          block
        />
      </div>

      <div class="form-row flex flex-col gap-1.5">
        <label class="form-label text-xs font-semibold text-slate-700 dark:text-slate-300">认证方式</label>
        <div class="radio-group flex items-center gap-4 text-xs text-slate-700 dark:text-slate-300">
          <label class="radio-label inline-flex items-center gap-1.5 cursor-pointer">
            <input v-model="authType" type="radio" value="password" class="text-blue-600 accent-blue-600" />
            <span>密码认证</span>
          </label>
          <label class="radio-label inline-flex items-center gap-1.5 cursor-pointer">
            <input v-model="authType" type="radio" value="key" class="text-blue-600 accent-blue-600" />
            <span>私钥认证 (SSH Key)</span>
          </label>
        </div>
      </div>

      <div v-if="authType === 'password'" class="form-row flex flex-col gap-1.5">
        <label class="form-label text-xs font-semibold text-slate-700 dark:text-slate-300" for="server-password">
          密码 <span v-if="!isEditing" class="text-red-500">*</span>
          <span v-else class="hint-text text-slate-400 font-normal">（留空则保持原密码不变）</span>
        </label>
        <div class="password-wrapper flex items-center gap-2">
          <UiInput
            id="server-password"
            v-model="secret"
            :type="showPassword ? 'text' : 'password'"
            placeholder="输入 SSH 连接密码"
            block
            class="flex-1"
          />
          <UiButton
            type="button"
            variant="ghost"
            size="md"
            class="btn-toggle-visibility"
            @click="showPassword = !showPassword"
          >
            {{ showPassword ? '隐藏' : '显示' }}
          </UiButton>
        </div>
      </div>

      <div v-else class="form-row flex flex-col gap-1.5">
        <label class="form-label text-xs font-semibold text-slate-700 dark:text-slate-300" for="server-key">
          SSH 私钥文本 <span v-if="!isEditing" class="text-red-500">*</span>
          <span v-else class="hint-text text-slate-400 font-normal">（留空则保持原私钥不变）</span>
        </label>
        <UiTextarea
          id="server-key"
          v-model="secret"
          :rows="5"
          class="key-textarea font-mono text-xs"
          placeholder="-----BEGIN OPENSSH PRIVATE KEY-----&#10;...&#10;-----END OPENSSH PRIVATE KEY-----"
        />
      </div>

      <UiAlert
        v-if="testResult"
        :variant="testResult.ok ? 'success' : 'error'"
        class="test-feedback"
      >
        {{ testResult.message }}
      </UiAlert>

      <div class="dialog-actions flex items-center justify-between gap-2.5 pt-3 border-t border-slate-100 dark:border-slate-800">
        <UiButton
          type="button"
          variant="secondary"
          class="btn-test"
          :disabled="testing"
          :loading="testing"
          @click="onTest"
        >
          {{ testing ? '测试中…' : '测试连接' }}
        </UiButton>

        <div class="flex items-center gap-2">
          <UiButton type="button" variant="secondary" class="btn-cancel" @click="emit('close')">
            取消
          </UiButton>
          <UiButton
            type="submit"
            variant="primary"
            class="btn-save"
            :disabled="saving"
            :loading="saving"
          >
            {{ saving ? '保存中…' : '保存' }}
          </UiButton>
        </div>
      </div>
    </form>
  </UiDialog>
</template>

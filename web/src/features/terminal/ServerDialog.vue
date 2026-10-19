<script setup lang="ts">
import { computed, ref } from 'vue'
import AppDialog from '../../shared/components/AppDialog.vue'
import { testServerConnection, type TerminalServer } from '../../api/terminal'
import { useTerminalStore } from '../../stores/terminal'
import { useAuthStore } from '../../stores/auth'

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
    const res = await testServerConnection(authStore.csrfToken, {
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
      saved = await terminalStore.updateServer(authStore.csrfToken, props.server.id, {
        name: name.value.trim(),
        host: host.value.trim(),
        port: Number(port.value) || 22,
        username: username.value.trim(),
        auth_type: authType.value,
        secret: secret.value ? secret.value : undefined,
      })
    } else {
      saved = await terminalStore.createServer(authStore.csrfToken, {
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
  <AppDialog :title="title" @close="emit('close')">
    <form class="server-form" @submit.prevent="onSubmit">
      <div v-if="errorMessage" class="form-error" role="alert">
        {{ errorMessage }}
      </div>

      <div class="form-row">
        <label class="form-label" for="server-name">
          服务器名称 <span class="required">*</span>
        </label>
        <input
          id="server-name"
          v-model="name"
          type="text"
          class="form-input"
          placeholder="例如：开发环境、数据库服务器"
          autofocus
        />
      </div>

      <div class="form-grid">
        <div class="form-row">
          <label class="form-label" for="server-host">
            主机地址 (Host) <span class="required">*</span>
          </label>
          <input
            id="server-host"
            v-model="host"
            type="text"
            class="form-input"
            placeholder="IP 或域名"
          />
        </div>

        <div class="form-row port-col">
          <label class="form-label" for="server-port">端口</label>
          <input
            id="server-port"
            v-model.number="port"
            type="number"
            class="form-input"
            min="1"
            max="65535"
            placeholder="22"
          />
        </div>
      </div>

      <div class="form-row">
        <label class="form-label" for="server-username">
          用户名 (Username) <span class="required">*</span>
        </label>
        <input
          id="server-username"
          v-model="username"
          type="text"
          class="form-input"
          placeholder="例如：root、ubuntu"
        />
      </div>

      <div class="form-row">
        <label class="form-label">认证方式</label>
        <div class="radio-group">
          <label class="radio-label">
            <input v-model="authType" type="radio" value="password" />
            <span>密码认证</span>
          </label>
          <label class="radio-label">
            <input v-model="authType" type="radio" value="key" />
            <span>私钥认证 (SSH Key)</span>
          </label>
        </div>
      </div>

      <div v-if="authType === 'password'" class="form-row">
        <label class="form-label" for="server-password">
          密码 <span v-if="!isEditing" class="required">*</span>
          <span v-else class="hint-text">（留空则保持原密码不变）</span>
        </label>
        <div class="password-wrapper">
          <input
            id="server-password"
            v-model="secret"
            :type="showPassword ? 'text' : 'password'"
            class="form-input"
            placeholder="输入 SSH 连接密码"
          />
          <button
            type="button"
            class="btn-toggle-visibility"
            @click="showPassword = !showPassword"
          >
            {{ showPassword ? '隐藏' : '显示' }}
          </button>
        </div>
      </div>

      <div v-else class="form-row">
        <label class="form-label" for="server-key">
          SSH 私钥文本 <span v-if="!isEditing" class="required">*</span>
          <span v-else class="hint-text">（留空则保持原私钥不变）</span>
        </label>
        <textarea
          id="server-key"
          v-model="secret"
          rows="5"
          class="form-textarea key-textarea"
          placeholder="-----BEGIN OPENSSH PRIVATE KEY-----&#10;...&#10;-----END OPENSSH PRIVATE KEY-----"
        />
      </div>

      <div v-if="testResult" class="test-feedback" :class="{ 'is-ok': testResult.ok }">
        <span class="test-icon">{{ testResult.ok ? '✓' : '✗' }}</span>
        <span>{{ testResult.message }}</span>
      </div>

      <div class="dialog-actions">
        <button
          type="button"
          class="btn-test"
          :disabled="testing"
          @click="onTest"
        >
          {{ testing ? '测试中…' : '测试连接' }}
        </button>

        <div class="spacer" />

        <button type="button" class="btn-cancel" @click="emit('close')">
          取消
        </button>
        <button
          type="submit"
          class="btn-save"
          :disabled="saving"
        >
          {{ saving ? '保存中…' : '保存' }}
        </button>
      </div>
    </form>
  </AppDialog>
</template>

<style scoped>
.server-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 440px;
}

.form-error {
  padding: 8px 12px;
  background: var(--color-danger-soft, rgba(239, 68, 68, 0.1));
  color: var(--color-danger, #ef4444);
  border: 1px solid var(--color-danger-border, rgba(239, 68, 68, 0.3));
  border-radius: 6px;
  font-size: 13px;
}

.form-row {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 100px;
  gap: 12px;
}

.form-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--color-text);
}

.required {
  color: #ef4444;
}

.hint-text {
  font-size: 12px;
  color: var(--color-text-muted);
  font-weight: normal;
}

.form-input,
.form-textarea {
  background: var(--color-surface-muted, #1e1e24);
  color: var(--color-text, #fff);
  border: 1px solid var(--color-border, #3a3a44);
  border-radius: 6px;
  padding: 8px 12px;
  font-size: 13px;
  transition: border-color 0.15s;
}

.form-input:focus,
.form-textarea:focus {
  outline: none;
  border-color: var(--color-primary, #3b82f6);
}

.password-wrapper {
  position: relative;
  display: flex;
  align-items: center;
}

.password-wrapper .form-input {
  width: 100%;
  padding-right: 50px;
}

.btn-toggle-visibility {
  position: absolute;
  right: 8px;
  background: transparent;
  border: none;
  color: var(--color-text-muted);
  font-size: 12px;
  cursor: pointer;
  padding: 4px;
}

.btn-toggle-visibility:hover {
  color: var(--color-text);
}

.key-textarea {
  font-family: var(--font-mono, monospace);
  font-size: 12px;
  line-height: 1.4;
  resize: vertical;
}

.radio-group {
  display: flex;
  gap: 20px;
}

.radio-label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--color-text);
  cursor: pointer;
}

.test-feedback {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-radius: 6px;
  font-size: 13px;
  background: var(--color-danger-soft, rgba(239, 68, 68, 0.1));
  color: var(--color-danger, #ef4444);
  border: 1px solid var(--color-danger-border, rgba(239, 68, 68, 0.3));
}

.test-feedback.is-ok {
  background: rgba(34, 197, 94, 0.1);
  color: #22c55e;
  border-color: rgba(34, 197, 94, 0.3);
}

.dialog-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 8px;
}

.spacer {
  flex: 1;
}

.btn-test {
  padding: 7px 12px;
  background: var(--color-surface-muted, #2a2b32);
  color: var(--color-text, #fff);
  border: 1px solid var(--color-border, #3f3f46);
  border-radius: 6px;
  font-size: 13px;
  cursor: pointer;
}

.btn-test:hover:not(:disabled) {
  background: var(--color-surface, #373842);
}

.btn-cancel {
  padding: 7px 14px;
  background: transparent;
  color: var(--color-text-muted, #a1a1aa);
  border: 1px solid var(--color-border, #3f3f46);
  border-radius: 6px;
  font-size: 13px;
  cursor: pointer;
}

.btn-cancel:hover {
  background: var(--color-surface, #27272a);
}

.btn-save {
  padding: 7px 16px;
  background: var(--color-primary, #2563eb);
  color: #fff;
  border: none;
  border-radius: 6px;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
}

.btn-save:hover:not(:disabled) {
  opacity: 0.9;
}

.btn-save:disabled,
.btn-test:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>

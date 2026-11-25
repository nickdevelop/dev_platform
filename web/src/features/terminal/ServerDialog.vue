<script setup lang="ts">
// ServerDialog: add or edit a third-party SSH server.
//
// `EntityFormDialog` + `el-form` own the chrome and the validation. Two things
// deliberately do *not* live in the footer: 测试连接 (the footer is fixed to
// 取消 / 确认 across the platform, so it hangs in the header instead) and the
// test verdict (it is a result about the address, so it stays inline as an
// `el-alert` rather than a toast that fades before you read it).
import { computed, reactive, ref } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { Connection } from '@element-plus/icons-vue'
import { testServerConnection, type TerminalServer } from '../../api/terminal'
import { useTerminalStore } from '../../stores/terminal'
import EntityFormDialog from '../../shared/components/EntityFormDialog.vue'

const props = defineProps<{
  server?: TerminalServer | null
}>()

const emit = defineEmits<{
  close: []
  saved: [server: TerminalServer]
}>()

const terminalStore = useTerminalStore()

const formRef = ref<FormInstance>()
const testing = ref(false)
const testResult = ref<{ ok: boolean; message: string } | null>(null)
const saving = ref(false)

const isEditing = computed(() => !!props.server && props.server.id > 0)

const form = reactive({
  name: props.server?.name ?? '',
  host: props.server?.host ?? '',
  port: props.server?.port ?? 22,
  username: props.server?.username ?? 'root',
  authType: (props.server?.auth_type === 'key' ? 'key' : 'password') as 'password' | 'key',
  secret: '',
})

const rules: FormRules = {
  name: [{ required: true, message: '请输入服务器名称', trigger: 'blur' }],
  host: [{ required: true, message: '请输入主机地址', trigger: 'blur' }],
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  // A new server needs a credential; an edit may leave it blank to keep the
  // stored one, so the rule follows `isEditing`.
  secret: [
    {
      validator: (_rule, value: string, callback) => {
        if (isEditing.value || value.trim()) callback()
        else callback(new Error(form.authType === 'password' ? '请输入密码' : '请输入私钥文本'))
      },
      trigger: 'blur',
    },
  ],
}

function payload() {
  return {
    name: form.name.trim(),
    host: form.host.trim(),
    port: Number(form.port) || 22,
    username: form.username.trim(),
    auth_type: form.authType,
  }
}

async function onTest() {
  if (testing.value) return
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  testing.value = true
  testResult.value = null
  try {
    const res = await testServerConnection({
      ...payload(),
      secret: form.secret || undefined,
      // Editing with a blank secret: ask the backend to use what it already
      // holds for this server instead of failing on an empty credential.
      server_id: isEditing.value && !form.secret ? props.server?.id : undefined,
    })
    testResult.value = { ok: true, message: res.message || '连接成功' }
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
  if (saving.value) return
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  saving.value = true
  try {
    let saved: TerminalServer
    if (isEditing.value && props.server) {
      saved = await terminalStore.updateServer(props.server.id, {
        ...payload(),
        secret: form.secret ? form.secret : undefined,
      })
    } else {
      saved = await terminalStore.createServer({ ...payload(), secret: form.secret })
    }
    emit('saved', saved)
  } catch (err: unknown) {
    ElMessage.error(err instanceof Error ? err.message : '保存失败')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <EntityFormDialog
    :title="isEditing ? '编辑服务器' : '添加第三方服务器'"
    subtitle="配置远程 SSH 节点的主机、凭据与认证方式"
    :identifier="isEditing ? props.server?.id : null"
    :saving="saving"
    :width="620"
    save-test="btn-save-server"
    data-test="server-dialog"
    @save="onSubmit"
    @cancel="emit('close')"
  >
    <template #head-extra>
      <el-button
        :loading="testing"
        :icon="Connection"
        data-test="server-test-btn"
        @click="onTest"
      >
        测试连接
      </el-button>
    </template>

    <el-form ref="formRef" :model="form" :rules="rules" label-position="top" @submit.prevent="onSubmit">
      <section class="entity-form-section">
        <el-form-item label="服务器名称" prop="name">
          <el-input
            v-model="form.name"
            maxlength="40"
            placeholder="例如：开发环境、数据库服务器"
            data-test="server-name-input"
          />
        </el-form-item>

        <div class="entity-grid-2">
          <el-form-item label="主机地址（Host）" prop="host">
            <el-input
              v-model="form.host"
              placeholder="IP 或域名"
              data-test="server-host-input"
            />
          </el-form-item>
          <el-form-item label="端口">
            <el-input-number
              v-model="form.port"
              :min="1"
              :max="65535"
              :controls-position="'right'"
              class="w-full"
              data-test="server-port-input"
            />
          </el-form-item>
        </div>

        <el-form-item label="用户名（Username）" prop="username">
          <el-input
            v-model="form.username"
            placeholder="例如：root、ubuntu"
            data-test="server-username-input"
          />
        </el-form-item>
      </section>

      <section class="entity-form-section">
        <header class="entity-section-head">
          <h3 class="entity-section-title">认证凭据</h3>
          <p class="entity-section-desc">
            {{ isEditing
              ? '留空即保持已保存的凭据不变；填写新值会覆盖它。'
              : '凭据由服务端加密保存，界面不会明文回显。' }}
          </p>
        </header>

        <el-form-item label="认证方式">
          <el-radio-group v-model="form.authType" data-test="server-auth-type">
            <el-radio value="password">密码认证</el-radio>
            <el-radio value="key">私钥认证（SSH Key）</el-radio>
          </el-radio-group>
        </el-form-item>

        <el-form-item v-if="form.authType === 'password'" label="密码" prop="secret">
          <el-input
            v-model="form.secret"
            type="password"
            show-password
            :placeholder="isEditing ? '留空则保持原密码不变' : '输入 SSH 连接密码'"
            data-test="server-secret-input"
          />
        </el-form-item>

        <el-form-item v-else label="SSH 私钥文本" prop="secret">
          <el-input
            v-model="form.secret"
            type="textarea"
            :rows="5"
            resize="vertical"
            class="entity-mono-input"
            placeholder="-----BEGIN OPENSSH PRIVATE KEY-----
...
-----END OPENSSH PRIVATE KEY-----"
            data-test="server-secret-input"
          />
        </el-form-item>

        <el-alert
          v-if="testResult"
          :type="testResult.ok ? 'success' : 'error'"
          :closable="false"
          show-icon
          :title="testResult.ok ? '连接成功' : '连接失败'"
          :description="testResult.message"
          data-test="server-test-result"
        />
      </section>
    </el-form>
  </EntityFormDialog>
</template>

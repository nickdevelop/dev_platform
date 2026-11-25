<script setup lang="ts">
// Connection settings: manage the local built-in backend and remote server
// connections, and switch which one the client talks to.
//
// Element Plus owns the surface: the connection roster is an `el-table`, the
// create / edit form moved into an `EntityFormDialog` (it used to swap the whole
// panel out for an inline form, which threw away the roster you were reading),
// and the switch-in-flight wait is `v-loading` over the roster instead of a
// spinner panel that replaced it. Failures go through `ElMessage` once; the
// web-mode notice stays an inline `el-alert` because it is an explanation, not a
// failure.
//
// The switch sequence itself is untouched and load-bearing: reset the
// multiplexed bus id (the old backend owns the persisted one), drop in-memory
// tabs, reset the auth session, then re-enter home so the new environment
// re-reads its own state.
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { Connection, Edit, Link, Monitor, Plus, Delete } from '@element-plus/icons-vue'
import {
  useDesktopConnection,
  type ServerConnection,
  type TestConnectionResult,
} from '../../shared/desktop'
import { useAuthStore } from '../../stores/auth'
import { useOpenTabsStore } from '../../stores/openTabs'
import { unifiedEventBus } from '../../shared/events/UnifiedEventBusClient'
import EntityFormDialog from '../../shared/components/EntityFormDialog.vue'
import ListEmptyState from '../../shared/components/ListEmptyState.vue'

const router = useRouter()
const authStore = useAuthStore()
const openTabsStore = useOpenTabsStore()

const {
  isDesktop,
  status,
  connections,
  activeConnection,
  switchTarget,
  save,
  remove,
  test,
  refresh,
} = useDesktopConnection()

const isSwitching = ref(false)

// ---------------------------------------------------------------- roster ----
const localAddress = computed(() => `本地内置单机服务 (127.0.0.1:${status.value?.localService.port || '动态'})`)

/** The badge a row carries: the active backend outranks "the local service
    happens to be running"; a remote row with no badge is simply idle. */
function stateTag(conn: ServerConnection): { type: 'primary' | 'success'; text: string } | null {
  if (conn.id === activeConnection.value?.id) return { type: 'primary', text: '当前激活' }
  if (conn.type === 'local' && status.value?.localService.running) return { type: 'success', text: '本地运行中' }
  return null
}

// ------------------------------------------------------------------ form ----
const formVisible = ref(false)
const formRef = ref<FormInstance>()
const saving = ref(false)
const isTesting = ref(false)
const testResult = ref<TestConnectionResult | null>(null)
const form = reactive({
  id: '' as string | undefined,
  name: '',
  url: '',
  token: '',
})

const isEditing = computed(() => !!form.id)

const rules: FormRules = {
  name: [{ required: true, message: '请输入服务器名称', trigger: 'blur' }],
  url: [
    { required: true, message: '请输入服务地址', trigger: 'blur' },
    {
      validator: (_rule, value: string, callback) => {
        if (!value || /^https?:\/\//.test(value.trim())) callback()
        else callback(new Error('请以 http:// 或 https:// 开头'))
      },
      trigger: 'blur',
    },
  ],
}

function openCreateForm() {
  form.id = undefined
  form.name = ''
  form.url = 'http://'
  form.token = ''
  testResult.value = null
  formVisible.value = true
}

function openEditForm(conn: ServerConnection) {
  form.id = conn.id
  form.name = conn.name
  form.url = conn.url || ''
  form.token = ''
  testResult.value = null
  formVisible.value = true
}

async function runTestConnection() {
  if (!/^https?:\/\//.test(form.url.trim())) {
    ElMessage.warning('请输入合法的 HTTP/HTTPS 服务器地址')
    return
  }
  isTesting.value = true
  testResult.value = null
  try {
    testResult.value = await test(form.url.trim(), form.token.trim() || undefined)
  } catch (err: any) {
    testResult.value = { ok: false, message: err?.message || '连接失败' }
  } finally {
    isTesting.value = false
  }
}

async function submit() {
  if (saving.value) return
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  saving.value = true
  try {
    await save({
      id: form.id,
      name: form.name.trim(),
      type: 'remote',
      url: form.url.trim(),
      token: form.token.trim() || undefined,
    })
    ElMessage.success(isEditing.value ? '已更新远程服务连接' : '已添加远程服务连接')
    formVisible.value = false
  } catch (err: any) {
    ElMessage.error(err?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

// ---------------------------------------------------------------- actions ---
async function removeConnection(conn: ServerConnection) {
  try {
    await ElMessageBox.confirm(conn.url || '', `确定要删除连接「${conn.name}」吗？`, {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      confirmButtonType: 'danger',
    })
  } catch {
    return
  }
  try {
    const ok = await remove(conn.id)
    if (ok === false) {
      ElMessage.error('删除远程连接失败')
      return
    }
    ElMessage.success(`已删除连接「${conn.name}」`)
  } catch (err: any) {
    ElMessage.error(err?.message || '删除远程连接失败')
  }
}

async function handleSwitch(conn: ServerConnection) {
  if (conn.id === activeConnection.value?.id || isSwitching.value) return
  isSwitching.value = true
  try {
    await switchTarget(conn.id)
    // Reset the multiplexed event stream: its connection (and persisted
    // busId) belongs to the previous backend environment — the main process
    // has already hard-cut the old pipes, so drop all client state and let
    // pages re-subscribe against the new environment.
    unifiedEventBus.resetBusId()
    // Clear in-memory tabs and auth session
    openTabsStore.tabs = []
    authStore.reset()
    // Force navigate to home to reload target environment state
    await router.push({ name: 'home' })
  } catch (err: any) {
    ElMessage.error(err?.message || '切换后端连接失败')
  } finally {
    isSwitching.value = false
  }
}
</script>

<template>
  <section class="connection-settings space-y-4" aria-label="服务连接设置">
    <header class="settings-page-head">
      <h1 class="settings-page-title">服务连接</h1>
      <p class="settings-page-desc">
        管理当前客户端连接的后端服务，支持在本地内置单机引擎与远程团队服务器间无缝切换
      </p>
    </header>

    <!-- Web browser environment callout: an explanation, not a failure, so it
         stays inline rather than becoming a toast. -->
    <el-alert
      v-if="!isDesktop"
      type="info"
      :closable="false"
      show-icon
      title="当前处于 Web 浏览器直连模式"
      description="您当前正直接访问宿主 Web 服务。如需在本地内置单机引擎与多台远程团队服务器间自由切换连接，请使用桌面客户端运行本平台。"
      data-test="web-mode-notice"
    />

    <section
      v-else
      v-loading="isSwitching"
      class="settings-panel"
      :element-loading-text="'正在切换目标后端，请稍候…'"
      aria-labelledby="connection-list-title"
    >
      <header class="settings-panel-head settings-panel-head-split">
        <div>
          <h2 id="connection-list-title" class="settings-panel-title">可用后端连接</h2>
          <p class="settings-panel-desc">
            桌面端当前已注册的本地与远程服务节点。激活后平台将自动指向所选服务。
          </p>
        </div>
        <el-button
          type="primary"
          :icon="Plus"
          class="shrink-0"
          data-test="btn-create-connection"
          @click="openCreateForm"
        >
          添加远程服务
        </el-button>
      </header>

      <el-table :data="connections" row-key="id" data-test="connection-table">
        <el-table-column label="服务名称" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <div class="flex items-center gap-2 min-w-0">
              <el-icon class="conn-glyph" :size="14">
                <component :is="row.type === 'local' ? Monitor : Link" />
              </el-icon>
              <span class="font-medium truncate">{{ row.name }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="部署形态" width="96">
          <template #default="{ row }">
            <el-tag class="entity-chip" :class="`entity-chip--${row.type}`" size="small" disable-transitions>
              {{ row.type === 'local' ? '本地' : '远程' }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="服务地址" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.type === 'local'">{{ localAddress }}</span>
            <code v-else class="path-code">{{ row.url }}</code>
          </template>
        </el-table-column>

        <el-table-column label="状态" width="108">
          <template #default="{ row }">
            <el-tag
              v-if="stateTag(row as ServerConnection)"
              :type="stateTag(row as ServerConnection)!.type"
              size="small"
              disable-transitions
            >
              {{ stateTag(row as ServerConnection)!.text }}
            </el-tag>
            <span v-else class="text-platform-text-muted">未激活</span>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="150" align="right" fixed="right">
          <template #default="{ row }">
            <div class="inline-flex items-center gap-1">
              <el-button
                v-if="row.id !== activeConnection?.id"
                link
                type="primary"
                size="small"
                :disabled="isSwitching"
                :data-test="`btn-connect-${row.id}`"
                @click="handleSwitch(row as ServerConnection)"
              >
                连接
              </el-button>
              <template v-if="row.type === 'remote'">
                <el-button
                  link
                  type="primary"
                  size="small"
                  :icon="Edit"
                  :disabled="isSwitching"
                  :data-test="`btn-edit-connection-${row.id}`"
                  @click="openEditForm(row as ServerConnection)"
                >
                  编辑
                </el-button>
                <el-button
                  link
                  type="danger"
                  size="small"
                  :icon="Delete"
                  :disabled="isSwitching"
                  :data-test="`btn-delete-connection-${row.id}`"
                  @click="removeConnection(row as ServerConnection)"
                >
                  删除
                </el-button>
              </template>
            </div>
          </template>
        </el-table-column>

        <template #empty>
          <ListEmptyState
            state="empty"
            :icon="Connection"
            title="尚未登记任何后端连接"
            hint="桌面端默认只带本地内置单机服务；添加远程团队服务器后即可在两处环境间切换。"
          >
            <el-button type="primary" :icon="Plus" @click="openCreateForm">添加远程服务</el-button>
          </ListEmptyState>
        </template>
      </el-table>
    </section>

    <EntityFormDialog
      v-if="formVisible"
      :title="isEditing ? '编辑远程服务器' : '添加远程服务器'"
      subtitle="配置远程 dev_platform 服务端点地址与安全访问凭据"
      :saving="saving"
      :width="620"
      save-test="btn-save-connection"
      data-test="connection-edit-dialog"
      @save="submit"
      @cancel="formVisible = false"
    >
      <template #head-extra>
        <el-button
          :loading="isTesting"
          :icon="Connection"
          data-test="btn-test-connection"
          @click="runTestConnection"
        >
          测试连通性
        </el-button>
      </template>

      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" @submit.prevent="submit">
        <section class="entity-form-section">
          <el-form-item label="服务器名称" prop="name">
            <el-input
              v-model="form.name"
              maxlength="40"
              placeholder="例如：开发测试服 / 团队工作站"
              data-test="input-connection-name"
            />
          </el-form-item>

          <el-form-item label="服务地址（URL）" prop="url">
            <el-input
              v-model="form.url"
              placeholder="http://192.168.1.100:8080"
              data-test="input-connection-url"
              @change="testResult = null"
            />
          </el-form-item>

          <el-form-item label="访问密码 / 凭据">
            <div class="w-full space-y-1">
              <el-input
                v-model="form.token"
                type="password"
                show-password
                placeholder="留空则保持原凭据或免密"
                data-test="input-connection-token"
              />
              <p class="entity-field-hint">可选。保存后由系统加密存储，不会明文回显。</p>
            </div>
          </el-form-item>

          <!-- The result of 测试连通性 is a verdict about the address, so it
               reads as an inline alert rather than a toast that fades away. -->
          <el-alert
            v-if="testResult"
            :type="testResult.ok ? 'success' : 'error'"
            :closable="false"
            show-icon
            :title="testResult.ok
              ? `连接成功${testResult.latencyMs !== undefined ? `（${testResult.latencyMs}ms）` : ''}`
              : '连接失败'"
            :description="testResult.message || ''"
            data-test="connection-test-result"
          />
        </section>
      </el-form>
    </EntityFormDialog>
  </section>
</template>

<style scoped>
.connection-settings {
  min-width: 0;
}

/* 部署形态的图标只说「本地还是远程」，不带状态色——状态由「状态」列负责。 */
.conn-glyph {
  flex: none;
  color: var(--color-text-muted);
}

.path-code {
  padding: 1px 5px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface-muted);
  font-family: var(--font-mono);
  font-size: var(--text-size-12);
  color: var(--color-text);
}
</style>

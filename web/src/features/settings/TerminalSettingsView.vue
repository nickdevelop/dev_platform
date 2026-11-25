<script setup lang="ts">
// Terminal and SSH server settings: terminal font, local host shell, and the
// remote SSH roster.
//
// Element Plus owns the surface. Three hand-rolled things carried real defects
// and are gone: the font presets were buttons whose selected state used the
// class `border-accent`, which does not exist in this project's Tailwind color
// map (only the `platform.*` namespace does), so nothing ever looked selected;
// the discovered-shell cards used `bg-accent/10`, same problem; and the font
// preview hardcoded `#121316` / `#e4e4e7`, which the token rule forbids. The
// remote list is now an `el-table` with the shared three-state empty slot, so a
// first load, an empty roster and a filter that matched nothing no longer look
// identical.
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Box, Monitor, Plus, Refresh, Search, SetUp } from '@element-plus/icons-vue'
import { useTerminalStore } from '../../stores/terminal'
import { useProjectTerminalStore } from '../../stores/projectTerminal'
import { type TerminalServer, testServerConnection } from '../../api/terminal'
import ServerDialog from '../terminal/ServerDialog.vue'
import ListEmptyState from '../../shared/components/ListEmptyState.vue'

const terminalStore = useTerminalStore()
const projectTerminal = useProjectTerminalStore()

const searchKeyword = ref('')
const loading = ref(false)

const showDialog = ref(false)
const editingServer = ref<TerminalServer | null>(null)

// Connection test state, keyed by server id: each row tests independently.
const testingServerId = ref<number | null>(null)
const testResults = reactive<Record<number, { ok: boolean; message: string }>>({})

// Local shell configuration.
const customShellPath = ref('')
const customShellArgs = ref('')
const isSavingShell = ref(false)
const isValidatingShell = ref(false)
const shellValidation = ref<{ ok: boolean; message: string } | null>(null)

// Terminal font configuration.
const customFontFamily = ref(terminalStore.fontFamily || '')

const PLATFORM_FONT = '__platform__'

const FONT_PRESETS = [
  { label: '平台统一字体（内置）', value: PLATFORM_FONT },
  { label: 'JetBrains Mono', value: 'JetBrains Mono' },
  { label: 'Fira Code', value: 'Fira Code' },
  { label: 'Cascadia Code', value: 'Cascadia Code' },
  { label: 'Consolas', value: 'Consolas' },
  { label: 'Monaco', value: 'Monaco' },
  { label: 'Hack', value: 'Hack' },
  { label: 'Source Code Pro', value: 'Source Code Pro' },
]

/** The select mirrors the free-text field: a value that is not one of the
    presets shows as "custom", an empty one as the platform face. */
const fontPreset = computed(() => {
  const value = customFontFamily.value.trim()
  if (!value) return PLATFORM_FONT
  return FONT_PRESETS.some((p) => p.value === value) ? value : ''
})

function onFontPresetChange(value: string) {
  customFontFamily.value = value === PLATFORM_FONT ? '' : value
}

// The font list is a free-text field with suggestions, so a value that matches
// no preset still has to be selectable in the dropdown instead of vanishing.
const fontOptions = computed(() => {
  const current = customFontFamily.value.trim()
  const known = FONT_PRESETS.map((p) => p.value)
  if (current && !known.includes(current)) {
    return [...FONT_PRESETS, { label: `${current}（自定义）`, value: current }]
  }
  return FONT_PRESETS
})

async function onSaveFont() {
  try {
    await terminalStore.saveTerminalFontFamily(customFontFamily.value)
    ElMessage.success(
      customFontFamily.value.trim()
        ? `已保存自定义终端字体：${customFontFamily.value.trim()}`
        : '已重置为平台统一内置字体',
    )
  } catch (err: unknown) {
    ElMessage.error(err instanceof Error ? err.message : '保存终端字体失败')
  }
}

async function onResetFont() {
  try {
    await terminalStore.resetTerminalFontFamily()
    customFontFamily.value = ''
    ElMessage.success('已恢复平台统一内置字体')
  } catch (err: unknown) {
    ElMessage.error(err instanceof Error ? err.message : '重置终端字体失败')
  }
}

async function fetchData() {
  loading.value = true
  try {
    await Promise.all([
      terminalStore.loadStatus(),
      terminalStore.loadServers(),
      terminalStore.loadLocalShell(),
      terminalStore.loadTerminalConfig(),
    ])
    customFontFamily.value = terminalStore.fontFamily || ''
    if (terminalStore.localShell) {
      customShellPath.value = terminalStore.localShell.path || ''
      customShellArgs.value = (terminalStore.localShell.args || []).join(' ')
    }
  } catch (err: unknown) {
    ElMessage.error(err instanceof Error ? err.message : '加载终端服务与配置失败')
  } finally {
    loading.value = false
  }
}

async function onScanShells() {
  shellValidation.value = null
  try {
    const list = await terminalStore.scanShells()
    if (list.length > 0) ElMessage.success(`扫描完成，发现 ${list.length} 个可用本地终端`)
    else shellValidation.value = { ok: false, message: '未扫描到可用 Shell' }
  } catch (err: unknown) {
    ElMessage.error(err instanceof Error ? err.message : '扫描失败')
  }
}

/** Picking a discovered shell fills both fields; the radio group's own value is
    the path, so the selection tracks what is actually configured. */
function onShellPicked(path: string) {
  const shell = terminalStore.discoveredShells.find((s) => s.path === path)
  if (!shell) return
  customShellPath.value = shell.path
  customShellArgs.value = (shell.args || []).join(' ')
  shellValidation.value = null
}

async function onValidateShell() {
  if (!customShellPath.value.trim()) {
    shellValidation.value = { ok: false, message: '请先输入 Shell 路径' }
    return
  }
  isValidatingShell.value = true
  shellValidation.value = null
  try {
    const res = await terminalStore.validateLocalShell(customShellPath.value.trim())
    shellValidation.value = { ok: res.ok, message: res.message || '路径有效且可执行' }
  } catch (err: unknown) {
    shellValidation.value = {
      ok: false,
      message: err instanceof Error ? err.message : '路径不可达或不可执行',
    }
  } finally {
    isValidatingShell.value = false
  }
}

async function onSaveShell() {
  if (!customShellPath.value.trim()) {
    shellValidation.value = { ok: false, message: 'Shell 路径不能为空' }
    return
  }
  isSavingShell.value = true
  shellValidation.value = null
  try {
    const args = customShellArgs.value.trim() ? customShellArgs.value.trim().split(/\s+/) : []
    await terminalStore.saveLocalShell({ path: customShellPath.value.trim(), args })
    shellValidation.value = { ok: true, message: '本地终端 Shell 设置已保存并即时生效' }
  } catch (err: unknown) {
    ElMessage.error(err instanceof Error ? err.message : '保存本地终端设置失败')
  } finally {
    isSavingShell.value = false
  }
}

async function onResetShell() {
  isSavingShell.value = true
  shellValidation.value = null
  try {
    const reset = await terminalStore.resetLocalShell()
    customShellPath.value = reset.path || ''
    customShellArgs.value = (reset.args || []).join(' ')
    shellValidation.value = { ok: true, message: '已恢复系统默认 Shell 配置' }
  } catch (err: unknown) {
    ElMessage.error(err instanceof Error ? err.message : '恢复默认设置失败')
  } finally {
    isSavingShell.value = false
  }
}

onMounted(() => {
  void fetchData()
})

const localServer = computed(() =>
  terminalStore.servers.find((s) => s.is_local || s.id === 0),
)

// 搜索是即时过滤：SSH 数量是个位数到几十的量，加一对「搜索 / 重置」按钮只会让
// 用户多按一次（画布内的表格不属于列表页工具条契约，见 v2 §四）。
const remoteServers = computed(() => {
  const remotes = terminalStore.servers.filter((s) => !s.is_local && s.id !== 0)
  const q = searchKeyword.value.trim().toLowerCase()
  if (!q) return remotes
  return remotes.filter(
    (s) =>
      s.name.toLowerCase().includes(q) ||
      s.host.toLowerCase().includes(q) ||
      s.username.toLowerCase().includes(q),
  )
})

/** 空态三态：首次加载 / 筛不到 / 真的没有。 */
const emptyState = computed<'loading' | 'filtered' | 'empty'>(() => {
  if (loading.value && terminalStore.servers.length === 0) return 'loading'
  if (searchKeyword.value.trim()) return 'filtered'
  return 'empty'
})

function openCreateDialog() {
  editingServer.value = null
  showDialog.value = true
}

function openEditDialog(server: TerminalServer) {
  editingServer.value = server
  showDialog.value = true
}

async function removeServer(server: TerminalServer) {
  try {
    await ElMessageBox.confirm(
      '删除后，相关连接凭证将立即清除，已打开的会话将在关闭后不可再连接。',
      `确定要删除服务器「${server.name}」吗？`,
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消', confirmButtonType: 'danger' },
    )
  } catch {
    return
  }
  try {
    await terminalStore.deleteServer(server.id)
    ElMessage.success(`已删除服务器「${server.name}」`)
    delete testResults[server.id]
  } catch (err: unknown) {
    ElMessage.error(err instanceof Error ? err.message : '删除服务器失败')
  }
}

async function onTestServer(server: TerminalServer) {
  if (testingServerId.value !== null) return
  testingServerId.value = server.id
  delete testResults[server.id]
  try {
    const res = await testServerConnection({
      host: server.host,
      port: server.port,
      username: server.username,
      auth_type: server.auth_type === 'key' ? 'key' : 'password',
      server_id: server.id,
    })
    testResults[server.id] = { ok: res.ok, message: res.ok ? '连接测试成功' : res.message || '连接失败' }
  } catch (err: unknown) {
    testResults[server.id] = {
      ok: false,
      message: err instanceof Error ? err.message : '连接超时或握手失败',
    }
  } finally {
    testingServerId.value = null
  }
}

function openTerminalForServer(server: TerminalServer) {
  if (server.is_local || server.id === 0) {
    projectTerminal.openLocalTab()
  } else {
    projectTerminal.openServerTab(server.id, server.name)
  }
}

async function onDialogSaved() {
  showDialog.value = false
  await terminalStore.loadServers()
}
</script>

<template>
  <section class="terminal-settings-view space-y-4" aria-label="终端服务器设置">
    <header class="settings-page-head">
      <h1 class="settings-page-title">终端服务器</h1>
      <p class="settings-page-desc">
        管理本地终端环境与远程 SSH 服务器连接配置，支持快速发起终端会话及服务连通性测试
      </p>
    </header>

    <!-- 终端字体 -->
    <section class="settings-panel" aria-labelledby="terminal-font-title">
      <header class="settings-panel-head settings-panel-head-split">
        <div class="min-w-0">
          <div class="flex items-center gap-2">
            <h2 id="terminal-font-title" class="settings-panel-title">终端字体</h2>
            <el-tag
              :type="terminalStore.fontFamily ? 'primary' : 'info'"
              size="small"
              disable-transitions
            >
              {{ terminalStore.fontFamily ? '自定义字体' : '平台统一字体' }}
            </el-tag>
          </div>
          <p class="settings-panel-desc">
            默认采用平台统一字体（Google Sans Code + Noto Sans SC），也可选择或手动指定本机已安装的等宽字体。
          </p>
        </div>
      </header>

      <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
        <div>
          <label class="field-label" for="terminal-font-preset">常用等宽字体</label>
          <el-select
            id="terminal-font-preset"
            :model-value="fontPreset"
            placeholder="自定义（不在预设内）"
            class="w-full"
            data-test="font-preset-select"
            @change="onFontPresetChange"
          >
            <el-option
              v-for="preset in fontOptions"
              :key="preset.value"
              :label="preset.label"
              :value="preset.value"
              :data-test="`font-preset-${preset.value === PLATFORM_FONT ? 'default' : preset.value}`"
            />
          </el-select>
        </div>

        <div class="sm:col-span-2">
          <label class="field-label" for="terminal-font-input">字体族名称（Font Family）</label>
          <div class="flex items-center gap-2">
            <el-input
              id="terminal-font-input"
              v-model="customFontFamily"
              class="entity-mono-input flex-1"
              placeholder="留空即使用平台统一字体，或输入如：JetBrains Mono、Fira Code"
              data-test="terminal-font-input"
            />
            <el-button
              type="primary"
              :loading="terminalStore.fontSaving"
              data-test="save-font-btn"
              @click="onSaveFont"
            >
              保存字体
            </el-button>
            <el-button
              :disabled="terminalStore.fontSaving || (!terminalStore.fontFamily && !customFontFamily)"
              data-test="reset-font-btn"
              @click="onResetFont"
            >
              恢复默认
            </el-button>
          </div>
        </div>
      </div>

      <div class="pt-3">
        <span class="field-label">字体效果实时预览</span>
        <!-- 预览跟随主题，而不是把终端写死成深色：终端本身也随预设翻色，
             写死 #121316 在浅色预设下反而与真实效果不符。 -->
        <div
          class="font-preview font-mono"
          :style="{ fontFamily: terminalStore.effectiveFontFamily }"
          data-test="terminal-font-preview"
        >
          <div>$ echo "Hello 平台终端" &amp;&amp; git status</div>
          <div class="font-preview-ok">✓ 字符集测试: 0123456789 ABCDEF abcdef != &lt;= &gt;= -&gt; =&gt; // 中文字符正常显示</div>
        </div>
      </div>
    </section>

    <!-- 本地宿主环境 -->
    <section class="settings-panel" aria-labelledby="terminal-local-title">
      <header class="settings-panel-head settings-panel-head-split">
        <div class="flex items-center gap-2 flex-wrap min-w-0">
          <h2 id="terminal-local-title" class="settings-panel-title">本地宿主环境</h2>
          <span class="id-chip font-mono">{{ terminalStore.status?.os || 'unknown' }}</span>
          <el-tag
            :type="terminalStore.isSupported ? 'success' : 'warning'"
            size="small"
            disable-transitions
          >
            {{ terminalStore.isSupported ? 'PTY 引擎就绪' : '不支持本地 PTY' }}
          </el-tag>
        </div>
        <el-button
          :disabled="!terminalStore.isSupported"
          :icon="SetUp"
          class="shrink-0"
          data-test="open-local-term-btn"
          @click="openTerminalForServer(localServer || { id: 0, name: '本机 (Localhost)', host: 'localhost', port: 0, username: 'local', auth_type: 'local', is_local: true, has_secret: false })"
        >
          打开本机终端
        </el-button>
      </header>

      <el-alert
        v-if="!terminalStore.isSupported"
        type="warning"
        :closable="false"
        show-icon
        title="当前操作系统环境不支持本地 PTY 终端"
        :description="terminalStore.status?.message || '您仍可添加并连接远程 SSH 服务器。'"
      />

      <div v-else class="space-y-3">
        <div class="shell-summary">
          <div class="min-w-0">
            <div class="flex items-center gap-2 flex-wrap">
              <span class="text-platform-text-muted">当前生效 Shell</span>
              <span class="font-medium">{{ terminalStore.localShell?.name || '默认终端' }}</span>
              <el-tag v-if="terminalStore.localShell?.is_recommended" type="success" size="small" disable-transitions>
                推荐
              </el-tag>
              <el-tag
                class="entity-chip"
                :class="terminalStore.localShell?.is_custom ? 'entity-chip--custom' : 'entity-chip--default'"
                size="small"
                disable-transitions
              >
                {{ terminalStore.localShell?.is_custom ? '自定义配置' : '系统默认' }}
              </el-tag>
            </div>
            <p class="shell-path font-mono">
              {{ terminalStore.localShell?.path || '-' }}
              <span v-if="terminalStore.localShell?.args?.length">
                · 参数 {{ terminalStore.localShell.args.join(' ') }}
              </span>
            </p>
          </div>
          <el-button
            :loading="terminalStore.scanningShells"
            :icon="Search"
            class="shrink-0"
            data-test="scan-shells-btn"
            @click="onScanShells"
          >
            扫描本地 Shell
          </el-button>
        </div>

        <!-- 候选 Shell：单选语义，用 el-radio 承载卡片，选中即填入下方表单。 -->
        <div v-if="terminalStore.discoveredShells.length > 0">
          <p class="field-label">已探测到的本地 Shell 候选</p>
          <el-radio-group
            class="shell-grid"
            :model-value="customShellPath"
            @change="onShellPicked($event as string)"
          >
            <el-radio
              v-for="sh in terminalStore.discoveredShells"
              :key="sh.id + sh.path"
              :value="sh.path"
              border
              class="option-card shell-card"
              :data-test="`shell-card-${sh.id}`"
            >
              <div class="shell-card-body">
                <div class="flex items-center gap-1.5">
                  <span class="font-medium truncate">{{ sh.name }}</span>
                  <el-tag v-if="sh.isRecommended" type="success" size="small" disable-transitions>推荐</el-tag>
                </div>
                <span class="shell-card-path font-mono truncate" :title="sh.path">{{ sh.path }}</span>
                <span v-if="sh.args?.length" class="shell-card-args">参数 {{ sh.args.join(' ') }}</span>
              </div>
            </el-radio>
          </el-radio-group>
        </div>

        <div class="space-y-3 pt-1">
          <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <div class="sm:col-span-2">
              <label class="field-label" for="shell-path-input">可执行文件绝对路径</label>
              <el-input
                id="shell-path-input"
                v-model="customShellPath"
                class="entity-mono-input"
                placeholder="例如 C:\Program Files\Git\bin\bash.exe 或 /bin/bash"
                data-test="shell-path-input"
              />
            </div>
            <div>
              <label class="field-label" for="shell-args-input">启动参数（空格分隔）</label>
              <el-input
                id="shell-args-input"
                v-model="customShellArgs"
                class="entity-mono-input"
                placeholder="例如 --login -i"
                data-test="shell-args-input"
              />
            </div>
          </div>

          <div class="flex items-center justify-between flex-wrap gap-2">
            <div class="flex items-center gap-2">
              <el-button
                type="primary"
                :loading="isSavingShell"
                :disabled="!customShellPath.trim()"
                data-test="save-shell-btn"
                @click="onSaveShell"
              >
                保存配置
              </el-button>
              <el-button
                :loading="isValidatingShell"
                :disabled="!customShellPath.trim()"
                data-test="validate-shell-btn"
                @click="onValidateShell"
              >
                校验路径
              </el-button>
              <el-button
                :disabled="isSavingShell || !terminalStore.localShell?.is_custom"
                data-test="reset-shell-btn"
                @click="onResetShell"
              >
                恢复默认
              </el-button>
            </div>
          </div>

          <el-alert
            v-if="shellValidation"
            :type="shellValidation.ok ? 'success' : 'error'"
            :closable="false"
            show-icon
            :title="shellValidation.message"
            data-test="shell-validation"
          />
        </div>
      </div>
    </section>

    <!-- 远程 SSH 服务器 -->
    <section class="settings-panel" aria-labelledby="terminal-remote-title">
      <header class="settings-panel-head settings-panel-head-split">
        <div>
          <h2 id="terminal-remote-title" class="settings-panel-title">远程 SSH 服务器</h2>
          <p class="settings-panel-desc">配置用于远程维护、跨机部署与脚本调试的 SSH 节点。</p>
        </div>
        <div class="flex items-center gap-2 shrink-0">
          <el-input
            v-model="searchKeyword"
            :prefix-icon="Search"
            placeholder="名称 / 主机 / 用户"
            clearable
            class="ssh-search"
            data-test="ssh-search"
          />
          <el-button :loading="loading" :icon="Refresh" data-test="refresh-servers-btn" @click="fetchData">
            刷新
          </el-button>
          <el-button type="primary" :icon="Plus" data-test="create-server-btn" @click="openCreateDialog">
            新增服务器
          </el-button>
        </div>
      </header>

      <el-table :data="remoteServers" row-key="id" data-test="server-list">
        <el-table-column label="服务器名称" min-width="140" show-overflow-tooltip>
          <template #default="{ row }">
            <div class="flex items-center gap-2 min-w-0">
              <el-icon class="row-glyph" :size="14"><Monitor /></el-icon>
              <span class="font-medium truncate">{{ row.name }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="主机与端口" width="150">
          <template #default="{ row }">
            <code class="path-code font-mono">{{ row.host }}:{{ row.port }}</code>
          </template>
        </el-table-column>

        <el-table-column label="登录用户" width="96" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="font-mono">{{ row.username }}</span>
          </template>
        </el-table-column>

        <el-table-column label="认证方式" width="84">
          <template #default="{ row }">
            <el-tag class="entity-chip" :class="`entity-chip--${row.auth_type}`" size="small" disable-transitions>
              {{ row.auth_type === 'key' ? '私钥' : '密码' }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="连通性" min-width="130">
          <template #default="{ row }">
            <span v-if="testingServerId === row.id" class="text-platform-text-muted">检测中…</span>
            <el-tag
              v-else-if="testResults[row.id]"
              :type="testResults[row.id].ok ? 'success' : 'danger'"
              size="small"
              disable-transitions
            >
              {{ testResults[row.id].message }}
            </el-tag>
            <span v-else class="text-platform-text-muted">未测试</span>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="200" align="right" fixed="right">
          <template #default="{ row }">
            <div class="inline-flex items-center gap-1">
              <el-button
                link
                type="primary"
                size="small"
                data-test="open-server-term-btn"
                @click="openTerminalForServer(row as TerminalServer)"
              >
                终端
              </el-button>
              <el-button
                link
                type="primary"
                size="small"
                :disabled="testingServerId !== null"
                data-test="test-server-btn"
                @click="onTestServer(row as TerminalServer)"
              >
                测试
              </el-button>
              <el-button
                link
                type="primary"
                size="small"
                data-test="edit-server-btn"
                @click="openEditDialog(row as TerminalServer)"
              >
                编辑
              </el-button>
              <el-button
                link
                type="danger"
                size="small"
                data-test="delete-server-btn"
                @click="removeServer(row as TerminalServer)"
              >
                删除
              </el-button>
            </div>
          </template>
        </el-table-column>

        <template #empty>
          <ListEmptyState
            :state="emptyState"
            :icon="Box"
            :title="emptyState === 'filtered' ? '没有匹配的服务器' : '暂无配置的远程 SSH 服务器'"
            :hint="emptyState === 'filtered'
              ? `没有名称、主机或用户名包含「${searchKeyword}」的服务器。`
              : '添加服务器后，可直接在平台内通过 SSH 发起独立或多标签终端会话。'"
            :reset-label="emptyState === 'filtered' ? '清空搜索' : ''"
            @reset="searchKeyword = ''"
          >
            <el-button type="primary" :icon="Plus" data-test="create-first-server" @click="openCreateDialog">
              添加第一个远程服务器
            </el-button>
          </ListEmptyState>
        </template>
      </el-table>
    </section>

    <ServerDialog
      v-if="showDialog"
      :server="editingServer"
      @close="showDialog = false"
      @saved="onDialogSaved"
    />
  </section>
</template>

<style scoped>
.field-label {
  display: block;
  margin-bottom: 4px;
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
}

.font-preview {
  padding: 10px 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface-muted);
  color: var(--color-text);
  font-size: var(--text-size-12);
  line-height: 1.6;
  overflow-x: auto;
  user-select: none;
}

.font-preview-ok {
  color: var(--color-success);
}

.shell-summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  padding: 10px 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  font-size: var(--text-size-12);
}

.shell-path {
  margin: 3px 0 0;
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
  word-break: break-all;
}

.shell-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.shell-card {
  margin-right: 0;
  flex: 1 1 220px;
  max-width: 320px;
}

.shell-card-body {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.shell-card-path {
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
}

.shell-card-args {
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
}

.ssh-search {
  width: 190px;
}

.path-code {
  padding: 1px 5px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface-muted);
  font-size: var(--text-size-11);
  color: var(--color-text);
}

.id-chip {
  flex: none;
  padding: 0 5px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  font-size: var(--text-size-10);
  line-height: 16px;
  color: var(--color-text-muted);
}

.row-glyph {
  flex: none;
  color: var(--color-text-muted);
}
</style>

<script setup lang="ts">
// Service proxy settings: exposes server directories or local port HTTP services
// as transparent `/proxy/:alias/` ContextPath proxies.
//
// Element Plus owns the surface: the proxy list is an `el-table` (the hand-built
// article cards grew four different alignment tricks per row and could not
// line up a column with any neighbour page), enable/disable is an `el-switch`,
// the type is a categorical chip, and failures go through `ElMessage` once
// instead of a closable banner that survives into the next action.
//
// The table is the settings canvas variant: natural height, no `height="100%"`,
// no pagination — the canvas scrolls, so a viewport-anchored panel would fight
// it (see frontend-ui-system.md §四 设置中心画布例外).
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { CopyDocument, Link, Plus, Refresh, Share } from '@element-plus/icons-vue'
import { ProxyEditDialog } from '../proxies'
import ListEmptyState from '../../shared/components/ListEmptyState.vue'
import { copyToClipboard } from '../../shared/clipboard'
import { formatListTime } from '../../shared/format'
import {
  listProxies,
  deleteProxy,
  toggleProxy,
  type ServiceProxy,
} from '../../api/proxies'

const proxies = ref<ServiceProxy[]>([])
const loading = ref(false)
// The row whose enable switch is in flight: only that switch goes quiet, so a
// slow toggle on one proxy doesn't freeze the whole column.
const togglingId = ref<number | null>(null)

const isDialogOpen = ref(false)
const activeProxy = ref<ServiceProxy | null>(null)

function getFullUrl(alias: string): string {
  return `${window.location.origin}/proxy/${alias}/`
}

async function copyUrl(proxy: ServiceProxy) {
  const url = getFullUrl(proxy.alias)
  // The clipboard helper already carries the non-secure-context fallback, so the
  // page only reports the outcome instead of re-implementing a textarea hack.
  const ok = await copyToClipboard(url)
  if (ok) ElMessage.success(`已复制 ${url}`)
  else ElMessage.warning(`复制失败，请手动选取：${url}`)
}

function openUrl(proxy: ServiceProxy) {
  window.open(getFullUrl(proxy.alias), '_blank')
}

async function fetchProxies() {
  loading.value = true
  try {
    const list = await listProxies()
    proxies.value = Array.isArray(list) ? list : []
  } catch (err: any) {
    ElMessage.error(err?.error || err?.message || '加载代理列表失败')
  } finally {
    loading.value = false
  }
}

function openCreateDialog() {
  activeProxy.value = null
  isDialogOpen.value = true
}

function openEditDialog(proxy: ServiceProxy) {
  activeProxy.value = proxy
  isDialogOpen.value = true
}

async function handleToggle(proxy: ServiceProxy) {
  if (togglingId.value !== null) return
  togglingId.value = proxy.id
  // The switch is bound to `proxy.enabled`, which the server owns: flip it
  // optimistically and pull it back if the write fails, otherwise the control
  // would sit showing a state that never happened.
  const previous = proxy.enabled
  proxy.enabled = !previous
  try {
    const updated = await toggleProxy(proxy.id)
    const idx = proxies.value.findIndex((p) => p.id === proxy.id)
    if (idx !== -1) proxies.value[idx] = updated
  } catch (err: any) {
    proxy.enabled = previous
    ElMessage.error(err?.error || err?.message || '切换代理状态失败')
  } finally {
    togglingId.value = null
  }
}

async function removeProxy(proxy: ServiceProxy) {
  try {
    await ElMessageBox.confirm(
      `别名 ${proxy.alias} 删除后，原 /proxy/${proxy.alias}/ 访问地址将立即失效。`,
      `确定要删除代理「${proxy.name}」吗？`,
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消', confirmButtonType: 'danger' },
    )
  } catch {
    return
  }
  try {
    await deleteProxy(proxy.id)
    proxies.value = proxies.value.filter((p) => p.id !== proxy.id)
    ElMessage.success(`已删除代理「${proxy.name}」`)
  } catch (err: any) {
    ElMessage.error(err?.error || err?.message || '删除代理失败')
  }
}

function onDialogSaved(saved: ServiceProxy) {
  isDialogOpen.value = false
  const idx = proxies.value.findIndex((p) => p.id === saved.id)
  if (idx !== -1) proxies.value[idx] = saved
  else proxies.value.unshift(saved)
}

onMounted(() => {
  fetchProxies()
})
</script>

<template>
  <section class="proxy-settings space-y-4" aria-label="服务代理设置">
    <header class="settings-page-head">
      <h1 class="settings-page-title">服务代理</h1>
      <p class="settings-page-desc">
        将服务器上的静态构建目录或本地端口 HTTP 服务以平台端口的独立二级路径（<code>/proxy/:alias/</code>）
        直接暴露访问，支持免登录分享与 WebSocket HMR 热更新
      </p>
    </header>

    <section class="settings-panel" aria-labelledby="proxy-list-title">
      <header class="settings-panel-head settings-panel-head-split">
        <div>
          <h2 id="proxy-list-title" class="settings-panel-title">代理规则</h2>
          <p class="settings-panel-desc">按 <code>/proxy/别名/</code> 暴露的访问路径与其映射目标</p>
        </div>
        <div class="flex items-center gap-2">
          <el-button :loading="loading" :icon="Refresh" data-test="btn-refresh-proxy" @click="fetchProxies">
            刷新
          </el-button>
          <el-button type="primary" :icon="Plus" data-test="btn-create-proxy" @click="openCreateDialog">
            新建代理
          </el-button>
        </div>
      </header>

      <el-table :data="proxies" row-key="id" data-test="proxy-table">
        <el-table-column label="代理名称" min-width="130" show-overflow-tooltip>
          <template #default="{ row }">
            <div class="flex items-center gap-2 min-w-0">
              <span class="font-medium">{{ row.name }}</span>
              <span class="id-chip font-mono">#{{ row.id }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="类型" width="84">
          <template #default="{ row }">
            <el-tag class="entity-chip" :class="`entity-chip--${row.type}`" size="small" disable-transitions>
              {{ row.type === 'directory' ? '静态目录' : '端口反代' }}
            </el-tag>
          </template>
        </el-table-column>

        <!-- 列宽预算：设置中心画布扣除侧栏与内边距后只有 ~985px，八列加起来
             必须装得下，否则 el-table 开横向滚动 + fixed="right" 会把中间的列
             盖到固定列后面（芯片被切一半、「启用 / 更新时间」直接看不见）。
             所以路径只留芯片，复制与打开改成图标链接（title / aria-label 说清范围）。 -->
        <el-table-column label="访问路径" min-width="150">
          <template #default="{ row }">
            <div class="flex items-center gap-1 min-w-0">
              <code class="path-code truncate" :title="getFullUrl(row.alias)">
                /proxy/{{ row.alias }}/
              </code>
              <el-button
                link
                type="primary"
                size="small"
                :icon="CopyDocument"
                title="复制访问链接"
                aria-label="复制访问链接"
                :data-test="`btn-copy-proxy-${row.id}`"
                @click="copyUrl(row as ServiceProxy)"
              />
              <el-button
                link
                type="primary"
                size="small"
                :icon="Link"
                title="在新窗口打开"
                aria-label="在新窗口打开"
                :disabled="!row.enabled"
                :data-test="`btn-open-proxy-${row.id}`"
                @click="openUrl(row as ServiceProxy)"
              />
            </div>
          </template>
        </el-table-column>

        <el-table-column label="映射目标" min-width="130" show-overflow-tooltip>
          <template #default="{ row }">
            <code class="path-code">{{ row.type === 'directory' ? row.target : `127.0.0.1:${row.target}` }}</code>
          </template>
        </el-table-column>

        <el-table-column label="端口状态" width="76">
          <template #default="{ row }">
            <el-tag
              v-if="row.type === 'port'"
              :type="row.online ? 'success' : 'info'"
              size="small"
              disable-transitions
            >
              {{ row.online ? '监听中' : '未连接' }}
            </el-tag>
            <span v-else class="text-platform-text-muted">—</span>
          </template>
        </el-table-column>

        <el-table-column label="启用" width="64" align="center">
          <template #default="{ row }">
            <el-switch
              :model-value="row.enabled"
              :loading="togglingId === row.id"
              :disabled="togglingId !== null"
              aria-label="启用此代理"
              :data-test="`proxy-enabled-${row.id}`"
              @change="handleToggle(row as ServiceProxy)"
            />
          </template>
        </el-table-column>

        <el-table-column label="更新时间" width="120" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="font-mono">{{ formatListTime(row.updatedAt) }}</span>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="100" align="right" fixed="right">
          <template #default="{ row }">
            <div class="inline-flex items-center gap-1">
              <el-button
                link
                type="primary"
                size="small"
                :data-test="`btn-edit-proxy-${row.id}`"
                @click="openEditDialog(row as ServiceProxy)"
              >
                编辑
              </el-button>
              <el-button
                link
                type="danger"
                size="small"
                :data-test="`btn-delete-proxy-${row.id}`"
                @click="removeProxy(row as ServiceProxy)"
              >
                删除
              </el-button>
            </div>
          </template>
        </el-table-column>

        <template #empty>
          <ListEmptyState
            :state="loading ? 'loading' : 'empty'"
            :icon="Share"
            title="暂无服务代理规则"
            hint="可以将服务器上的网页产物目录或本地运行的 Web 端口（如 Vite 5173、后端 8080）暴露为独立的 ContextPath，直接通过浏览器访问或分享。"
          >
            <el-button type="primary" :icon="Plus" @click="openCreateDialog">创建第一个代理</el-button>
          </ListEmptyState>
        </template>
      </el-table>
    </section>

    <ProxyEditDialog
      v-if="isDialogOpen"
      :proxy="activeProxy"
      @close="isDialogOpen = false"
      @saved="onDialogSaved"
    />
  </section>
</template>

<style scoped>
/* 表格里两处等宽小片：访问路径与映射目标。字号跟随数据单元格的 12px（v2 §三
   字号阶梯——数据单元格不另写更小的档）。 */
.path-code {
  padding: 1px 5px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface-muted);
  font-family: var(--font-mono);
  font-size: var(--text-size-12);
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
</style>

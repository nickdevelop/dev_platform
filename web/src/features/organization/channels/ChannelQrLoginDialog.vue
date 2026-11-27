<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import QRCode from 'qrcode'
import { ElMessage } from 'element-plus'
import { RefreshRight } from '@element-plus/icons-vue'
import type { BotChannel, QRLoginState } from '../../../api/bot'
import { useBotStore } from '../../../stores/bot'

const props = defineProps<{
  channel: BotChannel
}>()

const emit = defineEmits<{
  close: []
  connected: []
}>()

const botStore = useBotStore()
const starting = ref(false)
const qrDataUrl = ref('')
const renderError = ref('')
let pollTimer: number | null = null

const qrState = computed<QRLoginState | undefined>(
  () => botStore.qrStates[props.channel.id],
)

const statusText = computed(() => {
  const s = qrState.value?.status
  switch (s) {
    case 'pending':
      return '请使用个人微信扫描上方二维码并确认登录'
    case 'scanned':
      return '已扫码，请在微信手机端点击「确认」完成授权…'
    case 'confirmed':
      return '扫码授权成功，渠道已上线连接！'
    case 'expired':
      return '二维码已过期，请点击下方按钮重新获取'
    case 'cancelled':
      return '登录已取消'
    case 'error':
      return qrState.value?.error || '扫码登录过程发生错误'
    default:
      return '正在请求微信 iLink 登录二维码…'
  }
})

async function renderQrImage(text: string) {
  if (!text) {
    qrDataUrl.value = ''
    return
  }
  try {
    renderError.value = ''
    qrDataUrl.value = await QRCode.toDataURL(text, {
      width: 220,
      margin: 1,
    })
  } catch {
    qrDataUrl.value = ''
    renderError.value = '本地渲染二维码失败'
  }
}

watch(
  () => qrState.value?.qrUrl || qrState.value?.qrCode || '',
  (val) => {
    void renderQrImage(val)
  },
  { immediate: true },
)

watch(
  () => qrState.value?.status,
  (status) => {
    if (status === 'confirmed') {
      stopPoll()
      ElMessage.success(`渠道「${props.channel.name}」微信扫码连接成功`)
      emit('connected')
    } else if (status === 'expired' || status === 'error' || status === 'cancelled') {
      stopPoll()
    }
  },
)

function stopPoll() {
  if (pollTimer !== null) {
    window.clearInterval(pollTimer)
    pollTimer = null
  }
}

function startPoll() {
  stopPoll()
  pollTimer = window.setInterval(() => {
    const s = qrState.value?.status
    if (s === 'pending' || s === 'scanned' || !s) {
      void botStore.refreshQRStatus(props.channel.id)
    }
  }, 2500)
}

async function beginLogin() {
  starting.value = true
  qrDataUrl.value = ''
  renderError.value = ''
  try {
    const res = await botStore.beginQRLogin(props.channel.id)
    if (!res) {
      ElMessage.error(botStore.error || '获取二维码失败')
      return
    }
    startPoll()
  } finally {
    starting.value = false
  }
}

async function handleCancel() {
  stopPoll()
  await botStore.cancelQRLogin(props.channel.id)
  emit('close')
}

onMounted(() => {
  void beginLogin()
})

onBeforeUnmount(() => {
  stopPoll()
  const s = qrState.value?.status
  if (s === 'pending' || s === 'scanned') {
    void botStore.cancelQRLogin(props.channel.id)
  }
})
</script>

<template>
  <el-dialog
    :model-value="true"
    width="420px"
    :close-on-click-modal="false"
    data-test="channel-qr-login-dialog"
    @close="handleCancel"
  >
    <template #header>
      <div class="qr-dialog-head">
        <div class="qr-dialog-title">微信扫码连接 · {{ channel.name }}</div>
        <div class="qr-dialog-sub">通过 iLink Bot 协议扫码授权个人微信</div>
      </div>
    </template>

    <div class="qr-dialog-body">
      <div class="qr-box">
        <div v-if="starting" class="qr-placeholder">
          <span>二维码生成中…</span>
        </div>
        <img
          v-else-if="qrDataUrl"
          :src="qrDataUrl"
          alt="微信登录二维码"
          class="qr-image"
        />
        <div v-else class="qr-placeholder">
          <span>{{ renderError || '等待二维码数据…' }}</span>
        </div>
      </div>

      <div class="qr-status">
        <el-tag
          v-if="qrState?.status === 'scanned'"
          type="warning"
          size="small"
        >
          已扫码
        </el-tag>
        <el-tag
          v-else-if="qrState?.status === 'confirmed'"
          type="success"
          size="small"
        >
          已连接
        </el-tag>
        <el-tag
          v-else-if="qrState?.status === 'expired' || qrState?.status === 'error'"
          type="danger"
          size="small"
        >
          {{ qrState?.status === 'expired' ? '已过期' : '失败' }}
        </el-tag>
        <span class="qr-status-text">{{ statusText }}</span>
      </div>
    </div>

    <template #footer>
      <div class="entity-dialog-actions qr-dialog-footer">
        <el-button @click="handleCancel">关闭</el-button>
        <el-button
          type="primary"
          :loading="starting"
          :icon="RefreshRight"
          @click="beginLogin"
        >
          刷新二维码
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<style scoped>
.qr-dialog-head {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.qr-dialog-title {
  font-size: var(--text-size-14);
  font-weight: 600;
  color: var(--color-text);
}

.qr-dialog-sub {
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
}

.qr-dialog-body {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 14px;
  padding: 8px 0;
}

.qr-box {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 240px;
  height: 240px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  overflow: hidden;
}

.qr-image {
  display: block;
  width: 220px;
  height: 220px;
}

.qr-placeholder {
  font-size: var(--text-size-12);
  color: var(--color-text-muted);
  text-align: center;
  padding: 0 16px;
}

.qr-status {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  text-align: center;
}

.qr-status-text {
  font-size: var(--text-size-12);
  color: var(--color-text-muted);
  line-height: 1.5;
}

.qr-dialog-footer {
  justify-content: flex-end;
  width: 100%;
}
</style>

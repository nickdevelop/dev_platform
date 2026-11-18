<script setup lang="ts">
// Login view: collects the shared platform password. On first run from localhost
// when no password is set, prompts the user to initialize the platform password.
// On success the router guard lets the user through to the home view.
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { describeApiError } from '../api/client'
import { useAuthStore } from '../stores/auth'
import { UiButton, UiInput, UiAlert } from '../shared/ui'

const router = useRouter()
const auth = useAuthStore()

const password = ref('')
const confirmPassword = ref('')
const errorMessage = ref('')
const submitting = ref(false)

async function submit() {
  if (submitting.value || password.value === '') return
  if (auth.needsSetup) {
    if (confirmPassword.value === '') return
    if (password.value !== confirmPassword.value) {
      errorMessage.value = '两次输入的密码不一致，请重新输入。'
      return
    }
  }

  submitting.value = true
  errorMessage.value = ''
  try {
    if (auth.needsSetup) {
      await auth.setupPassword(password.value)
    } else {
      await auth.login(password.value)
    }
    password.value = ''
    confirmPassword.value = ''
    await router.push({ name: 'home' })
  } catch (error) {
    errorMessage.value = describeApiError(
      error,
      auth.needsSetup ? '设置密码失败，请重试。' : '登录失败，请重试。',
    )
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="login-wrapper min-h-screen w-full flex flex-col items-center justify-center p-4 bg-platform-bg text-platform-text selection:bg-platform-primary-soft selection:text-platform-primary">
    <main class="login w-full max-w-sm rounded-xl border border-platform-border bg-platform-surface shadow-lg overflow-hidden transition-all duration-200">
      <!-- Card Header with Brand / Logo -->
      <div class="px-6 pt-7 pb-5 text-center border-b border-platform-border/60 bg-platform-surface-muted/40">
        <div class="inline-flex items-center justify-center w-12 h-12 rounded-xl bg-platform-primary/10 text-platform-primary border border-platform-primary/20 mb-3.5 shadow-sm">
          <svg viewBox="0 0 24 24" class="w-6 h-6" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="4 17 10 11 4 5" />
            <line x1="12" y1="19" x2="20" y2="19" />
          </svg>
        </div>
        <h1 class="text-lg font-semibold tracking-tight text-platform-text">AI 开发项目管理平台</h1>
        <p v-if="auth.needsSetup" class="text-xs text-platform-text-muted mt-1 leading-relaxed">
          检测到首次在本机启动，请设置初始访问密码。
        </p>
        <p v-else class="text-xs text-platform-text-muted mt-1 leading-relaxed">
          工程工作台与智能体协同环境
        </p>
      </div>

      <!-- Card Form -->
      <div class="p-6">
        <form class="login-form flex flex-col gap-4 text-left" @submit.prevent="submit">
          <div class="flex flex-col gap-1.5">
            <label for="login-password" class="text-xs font-medium text-platform-text">
              {{ auth.needsSetup ? '设置平台密码' : '平台密码' }}
            </label>
            <UiInput
              id="login-password"
              v-model="password"
              type="password"
              :autocomplete="auth.needsSetup ? 'new-password' : 'current-password'"
              :disabled="submitting"
              required
              block
              :placeholder="auth.needsSetup ? '请输入新的平台密码' : '请输入平台密码'"
            />
          </div>

          <template v-if="auth.needsSetup">
            <div class="flex flex-col gap-1.5">
              <label for="login-confirm-password" class="text-xs font-medium text-platform-text">
                确认密码
              </label>
              <UiInput
                id="login-confirm-password"
                v-model="confirmPassword"
                type="password"
                autocomplete="new-password"
                :disabled="submitting"
                required
                block
                placeholder="请再次输入平台密码"
              />
            </div>
          </template>

          <UiButton
            type="submit"
            variant="primary"
            block
            :disabled="submitting || password === '' || (auth.needsSetup && confirmPassword === '')"
            :loading="submitting"
            class="mt-1 font-medium shadow-sm active:scale-[0.99] transition-transform"
          >
            {{ auth.needsSetup ? '设置密码并登录' : '登录' }}
          </UiButton>

          <UiAlert v-if="errorMessage" variant="error" class="login-error text-xs">
            {{ errorMessage }}
          </UiAlert>
        </form>
      </div>

      <!-- Card Subtle Footer -->
      <div class="px-6 py-3 border-t border-platform-border/60 bg-platform-surface-muted/30 flex items-center justify-between text-3xs text-platform-text-muted font-mono">
        <span>DEV_PLATFORM // HARNESS</span>
        <span class="inline-flex items-center gap-1.5">
          <span class="w-1.5 h-1.5 rounded-full bg-platform-success"></span>
          LOCAL ACTIVE
        </span>
      </div>
    </main>
  </div>
</template>

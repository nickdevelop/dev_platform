<script setup lang="ts">
// Login view: collects the shared platform password. On first run from localhost
// when no password is set, prompts the user to initialize the platform password.
// On success the router guard lets the user through to the home view.
// Unslopped per craft guidelines:
// - 4px grid rhythm, quiet chrome, zero resting shadows
// - Clean typography, crisp 1px borders, no text below 11px
// - Authentic developer terminal identity
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
  <div class="login-wrapper min-h-screen w-full flex flex-col items-center justify-center p-6 bg-platform-bg text-platform-text">
    <main class="login w-full max-w-sm rounded-xl border border-platform-border bg-platform-surface overflow-hidden">
      <!-- Card Header with Brand / Logo -->
      <header class="px-6 pt-8 pb-6 text-center border-b border-platform-border/60 bg-platform-surface-muted/30">
        <div class="inline-flex items-center justify-center w-10 h-10 rounded-lg bg-surface border border-border text-foreground mb-3.5 shadow-none">
          <svg viewBox="0 0 24 24" class="w-5 h-5 text-accent" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="4 17 10 11 4 5" />
            <line x1="12" y1="19" x2="20" y2="19" />
          </svg>
        </div>
        <h1 class="text-base font-semibold tracking-tight text-platform-text">AI 开发项目管理平台</h1>
        <p v-if="auth.needsSetup" class="text-xs text-platform-text-muted mt-1 leading-relaxed">
          检测到首次在本机启动，请设置初始访问密码
        </p>
        <p v-else class="text-xs text-platform-text-muted mt-1 leading-relaxed">
          工程工作台与智能体协同环境
        </p>
      </header>

      <!-- Card Form -->
      <div class="p-6">
        <form class="login-form flex flex-col gap-4 text-left" @submit.prevent="submit">
          <div class="flex flex-col gap-2">
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
            <div class="flex flex-col gap-2">
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
            class="mt-1 font-medium"
          >
            {{ auth.needsSetup ? '设置密码并登录' : '登录' }}
          </UiButton>

          <UiAlert v-if="errorMessage" variant="error" class="login-error text-xs">
            {{ errorMessage }}
          </UiAlert>
        </form>
      </div>

      <!-- Card Subtle Footer -->
      <footer class="px-6 py-3 border-t border-platform-border/60 bg-platform-surface-muted/20 flex items-center justify-between text-2xs text-platform-text-muted font-mono">
        <span>服务状态</span>
        <span class="inline-flex items-center gap-1.5 text-foreground">
          <span class="w-1.5 h-1.5 rounded-full bg-emerald-500" aria-hidden="true"></span>
          本地服务就绪
        </span>
      </footer>
    </main>
  </div>
</template>

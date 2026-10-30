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
  <main class="login max-w-xs mx-auto mt-[15vh] px-4 text-center">
    <h1 class="text-xl font-bold text-slate-800 dark:text-slate-100 mb-2 tracking-tight">AI 开发项目管理平台</h1>
    <p v-if="auth.needsSetup" class="text-xs text-slate-500 dark:text-slate-400 mb-6">
      检测到首次在本机启动，请设置初始访问密码。
    </p>
    <div v-else class="mb-6" />

    <form class="login-form flex flex-col gap-3 text-left" @submit.prevent="submit">
      <label for="login-password" class="text-sm font-semibold text-slate-700 dark:text-slate-300">
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

      <template v-if="auth.needsSetup">
        <label for="login-confirm-password" class="text-sm font-semibold text-slate-700 dark:text-slate-300">
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
      </template>

      <UiButton
        type="submit"
        variant="primary"
        block
        :disabled="submitting || password === '' || (auth.needsSetup && confirmPassword === '')"
        :loading="submitting"
        class="mt-1"
      >
        {{ auth.needsSetup ? '设置密码并登录' : '登录' }}
      </UiButton>
      <UiAlert v-if="errorMessage" variant="error" class="login-error mt-2">
        {{ errorMessage }}
      </UiAlert>
    </form>
  </main>
</template>

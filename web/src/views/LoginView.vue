<script setup lang="ts">
// Login view: collects the shared platform password. On success the router
// guard lets the user through to the projects view.
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { describeApiError } from '../api/client'
import { useAuthStore } from '../stores/auth'
import { UiButton, UiInput, UiAlert } from '../shared/ui'

const router = useRouter()
const auth = useAuthStore()

const password = ref('')
const errorMessage = ref('')
const submitting = ref(false)

async function submit() {
  if (submitting.value || password.value === '') return
  submitting.value = true
  errorMessage.value = ''
  try {
    await auth.login(password.value)
    password.value = ''
    await router.push({ name: 'home' })
  } catch (error) {
    errorMessage.value = describeApiError(error, '登录失败，请重试。')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <main class="login max-w-xs mx-auto mt-[15vh] px-4 text-center">
    <h1 class="text-xl font-bold text-slate-800 dark:text-slate-100 mb-6 tracking-tight">AI 开发项目管理平台</h1>
    <form class="login-form flex flex-col gap-3 text-left" @submit.prevent="submit">
      <label for="login-password" class="text-sm font-semibold text-slate-700 dark:text-slate-300">平台密码</label>
      <UiInput
        id="login-password"
        v-model="password"
        type="password"
        autocomplete="current-password"
        :disabled="submitting"
        required
        block
        placeholder="请输入平台密码"
      />
      <UiButton
        type="submit"
        variant="primary"
        block
        :disabled="submitting || password === ''"
        :loading="submitting"
        class="mt-1"
      >
        登录
      </UiButton>
      <UiAlert v-if="errorMessage" variant="error" class="login-error mt-2">
        {{ errorMessage }}
      </UiAlert>
    </form>
  </main>
</template>

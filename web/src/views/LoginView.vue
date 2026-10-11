<script setup lang="ts">
// Login view: collects the shared platform password. On success the router
// guard lets the user through to the projects view.
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { describeApiError } from '../api/client'
import { useAuthStore } from '../stores/auth'

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
  <main class="login">
    <h1>AI 开发项目管理平台</h1>
    <form class="login-form" @submit.prevent="submit">
      <label for="login-password">平台密码</label>
      <input
        id="login-password"
        v-model="password"
        type="password"
        autocomplete="current-password"
        :disabled="submitting"
        required
      />
      <button type="submit" :disabled="submitting || password === ''">登录</button>
      <p v-if="errorMessage" class="login-error" role="alert">{{ errorMessage }}</p>
    </form>
  </main>
</template>

<style scoped>
.login {
  max-width: 20rem;
  margin: 15vh auto 0;
  padding: 0 1rem;
  text-align: center;
}

.login-form {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  margin-top: 1.5rem;
  text-align: left;
}

.login-form label {
  font-weight: 600;
}

.login-form input {
  padding: 0.5rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  font-size: 1rem;
}

.login-form button {
  padding: 0.5rem;
  border: none;
  border-radius: 6px;
  background: var(--color-primary);
  color: var(--color-surface);
  font-size: 1rem;
  cursor: pointer;
}

.login-form button:disabled {
  background: var(--color-info-border);
  cursor: default;
}

.login-error {
  margin: 0;
  color: var(--color-danger);
}
</style>

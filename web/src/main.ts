import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { createAppRouter } from './router'
import { applyCachedAppearance } from './theme/apply'
import { installCSRF } from './api/client'
import { useAuthStore } from './stores/auth'
import './style.css'

// Paint the last known appearance before mounting so the first paint does not
// flash the default theme while the server config loads.
applyCachedAppearance()

// Pinia is installed before the router so the navigation guard and the
// session-expiry handler can resolve the auth store.
const pinia = createPinia()

// The HTTP transport owns the CSRF token: it reads the current one from the
// session store and refreshes it through the session endpoint. Registering the
// provider here (the composition root) keeps api/client.ts free of any store
// import, and runs before the router's first navigation can issue a request.
installCSRF({
  get: () => useAuthStore(pinia).csrfToken,
  refresh: () => useAuthStore(pinia).fetchSession(),
})

createApp(App).use(pinia).use(createAppRouter()).mount('#app')

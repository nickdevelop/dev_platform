import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { createAppRouter } from './router'
import './style.css'

// Pinia is installed before the router so the navigation guard and the
// session-expiry handler can resolve the auth store.
createApp(App).use(createPinia()).use(createAppRouter()).mount('#app')

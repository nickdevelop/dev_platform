import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { createAppRouter } from './router'
import { applyCachedAppearance } from './theme/apply'
import './style.css'

// Paint the last known appearance before mounting so the first paint does not
// flash the default theme while the server config loads.
applyCachedAppearance()

// Pinia is installed before the router so the navigation guard and the
// session-expiry handler can resolve the auth store.
createApp(App).use(createPinia()).use(createAppRouter()).mount('#app')

import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider } from 'react-router'
import { AppProviders } from './app/providers'
import { router } from './app/router'
import { installCSRF } from './api/client'
import { useAuthStore } from './stores/auth'
import { applyCachedAppearance } from './theme/apply'
import './styles/tokens.css'
import './styles/base.css'

// Paint the cached palette before React mounts, matching the Vue app's
// first-paint behavior.
applyCachedAppearance()

// The HTTP transport owns the CSRF token. Registering the provider at the
// composition root keeps api/client.ts free of any store import.
installCSRF({
  get: () => useAuthStore.getState().csrfToken,
  refresh: () => useAuthStore.getState().fetchSession(),
})

createRoot(document.getElementById('app')!).render(
  <StrictMode>
    <AppProviders>
      <RouterProvider router={router} />
    </AppProviders>
  </StrictMode>,
)

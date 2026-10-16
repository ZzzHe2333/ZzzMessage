import { createApp } from 'vue'
import App from './App.vue'
import './style.css'
import { registerSW } from 'virtual:pwa-register'

// Preserve IndexedDB and the existing Web Push subscription when upgrading.
let hadController = Boolean(navigator.serviceWorker?.controller)
let reloadingForUpgrade = false
navigator.serviceWorker?.addEventListener('controllerchange', () => {
  if (hadController && !reloadingForUpgrade) {
    reloadingForUpgrade = true
    window.location.reload()
  }
  hadController = true
})
registerSW({
  immediate: true,
  onNeedRefresh() {
    window.dispatchEvent(new Event('zzz:frontend-update-ready'))
  },
  onRegisteredSW(_swUrl, registration) {
    // At launch, check if a newer UI bundle is already on the server.
    // This does not clear service-worker registrations or push data.
    registration?.update().catch(() => {})
  }
})
createApp(App).mount('#app')

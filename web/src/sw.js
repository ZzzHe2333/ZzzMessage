/// <reference lib="webworker" />
import { precacheAndRoute, cleanupOutdatedCaches, createHandlerBoundToURL } from 'workbox-precaching'
import { NavigationRoute, registerRoute } from 'workbox-routing'

precacheAndRoute(self.__WB_MANIFEST)
cleanupOutdatedCaches()
registerRoute(new NavigationRoute(createHandlerBoundToURL('/index.html'), {
  denylist: [/^\/api\//]
}))

self.addEventListener('install', () => self.skipWaiting())
self.addEventListener('activate', event => event.waitUntil(self.clients.claim()))

// Safari requires EVERY push to display a visible notification.
// The payload has no message body and isn't cached.
self.addEventListener('push', event => {
  // The server supplies authorized recipient-specific previews; users may
  // disable previews in account settings. Fall back for old payloads/tests.
  let data = {}
  try { data = event.data?.json() || {} } catch {}
  const roomId = typeof data.roomId === 'string' ? data.roomId : ''
  const path = roomId ? '/message?room=' + encodeURIComponent(roomId) : '/message'
  const title = typeof data.title === 'string' && data.title ? data.title : 'ZzzMessage'
  const body = typeof data.body === 'string' && data.body ? data.body : '收到一条新消息'
  const tag = typeof data.id === 'string' && /^[0-9a-f]{32}$/.test(data.id)
    ? 'zzzmessage-' + data.id : 'zzzmessage-' + Date.now()
  event.waitUntil(self.registration.showNotification(title, {
    body,
    icon: '/icon-192.png',
    badge: '/icon-192.png',
    tag,
    data: { path, roomId }
  }))
})
self.addEventListener('notificationclick', event => {
  event.notification.close()
  event.waitUntil((async () => {
    const info = event.notification.data || {}
    const path = typeof info.path === 'string' && info.path.startsWith('/message')
      ? info.path : '/message'
    const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true })
    for (const windowClient of windows) {
      if (new URL(windowClient.url).origin === self.location.origin) {
        await windowClient.focus()
        if(info.roomId) windowClient.postMessage({type:'zzzmessage:open-room',roomId:info.roomId})
        return
      }
    }
    await self.clients.openWindow(path)
  })())
})

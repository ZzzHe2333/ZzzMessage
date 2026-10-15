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
  event.waitUntil(self.registration.showNotification('ZzzMessage', {
    body: '收到一条新消息',
    icon: '/icon-192.png',
    badge: '/icon-192.png',
    tag: 'zzzmessage-new-message',
    data: { path: '/' }
  }))
})
self.addEventListener('notificationclick', event => {
  event.notification.close()
  event.waitUntil((async () => {
    const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true })
    for (const windowClient of windows) {
      if (new URL(windowClient.url).origin === self.location.origin) {
        await windowClient.focus()
        return
      }
    }
    await self.clients.openWindow('/')
  })())
})

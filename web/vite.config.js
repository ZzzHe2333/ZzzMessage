import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { VitePWA } from 'vite-plugin-pwa'

export default defineConfig({
  plugins: [vue(), VitePWA({
    registerType: 'autoUpdate',
    includeAssets: ['apple-touch-icon.png'],
    manifest: {
      name: 'ZzzMessage 私人聊天', short_name: 'ZzzMessage',
      description: '邀请制私人聊天，消息不持久存储在服务器',
      theme_color: '#171d32', background_color: '#f4f5f9', display: 'standalone',
      start_url: '/', scope: '/',
      icons: [
        { src: '/icon-192.png', sizes: '192x192', type: 'image/png' },
        { src: '/icon-512.png', sizes: '512x512', type: 'image/png' }
      ]
    },
    workbox: {
      navigateFallback: '/index.html',
      // Cache only the static app shell; never cache authentication/API responses.
      globPatterns: ['**/*.{js,css,html,svg,png}'],
      navigateFallbackDenylist: [/^\/api\//]
    }
  })],
  server: { port: 58464, strictPort: true }
})

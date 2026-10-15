<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { keepMessage, keepMessages, loadMessages, loadAllMessages, clearRoom } from './localMessages'

const SERVER_KEY = 'zzzmessage.server'
const ROUTES = new Set(['/login','/setting','/message','/friend','/gift','/money'])
const initialPath = ROUTES.has(location.pathname) ? location.pathname : '/login'
const routePath = ref(initialPath)
const server = ref(localStorage.getItem(SERVER_KEY) || location.origin)
const editedServer = ref(server.value)
const configured = ref(false)
const checkedServer = ref(false)
const registering = ref(false)
const usernameInput = ref('')
const displayName = ref('')
const password = ref('')
const inviteCode = ref('')
const token = ref('')
const me = ref(null)
const rooms = ref([])
const users = ref([])
const selectedId = ref('lobby')
const messages = ref([])
const draft = ref('')
const connection = ref('offline')
const status = ref('')
const busy = ref(false)
const tab = ref('chats')
const groupName = ref('')
const groupMembers = ref([])
const newGroup = ref(false)
const showSettings = ref(false)
const mobileChat = ref(false)
const deviceID = ref('')
const syncStatus = ref('未请求同步')
const syncBusy = ref(false)
const historyChunkWaiters = new Map()
const wallet = ref({ gold:0, silver:0, copper:0, lastDaily:'' })
const giftCatalog = ref([])
const giftHistory = ref([])
const giftRecipient = ref('')
const giftBusy = ref(false)
const economyStatus = ref('')
let pendingGiftRequestID = ''
let pendingGiftItemID = ''
let pendingGiftRecipient = ''

const isNativeAndroid = Boolean(window.NativePushBridge)
const nativeManufacturer = isNativeAndroid ? window.NativePushBridge.manufacturer() : ''
const nativeSupported = ['xiaomi','redmi','vivo','iqoo'].includes(nativeManufacturer)
const nativePushStatus = ref('')
const nativePushBound = ref(false)
const nativePushBusy = ref(false)
const pushAvailable = ref(false)
const pushEnabled = ref(false)
const pushBusy = ref(false)
const pushPublicKey = ref('')
const pushStatus = ref('')
const onIOS = /iPhone|iPad|iPod/.test(navigator.userAgent)
const installedPWA = window.matchMedia('(display-mode: standalone)').matches || navigator.standalone === true
let pushRegistration = null
const chatScroll = ref(null)
let socket = null
let retryTimer = null
let reconnectDelay = 1500
let liveSession = 0

const activeRoom = computed(() => rooms.value.find(r => r.id === selectedId.value))
const roomLabel = r => r.type === 'direct' ? (r.members.find(x => x !== me.value?.username) || '私聊') : r.name
const serverName = computed(() => { try { return new URL(server.value).host } catch { return server.value } })
const sortedMessages = computed(() => [...messages.value].sort((a,b) => a.createdAt.localeCompare(b.createdAt)))
const selectedUsers = computed(() => users.value.filter(u => groupMembers.value.includes(u.username)))

function isPrivateLANHost(hostname) {
  const parts = hostname.split('.')
  if (parts.length !== 4 || parts.some(p => !/^[0-9]{1,3}$/.test(p) || Number(p) > 255)) return false
  const [a,b] = parts.map(Number)
  return a === 10 || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168)
}
function syncRoute(next) {
  routePath.value = next
  if (next === '/gift') void openGiftShop()
  if (next === '/money') void refreshWallet()
  if (next === '/gift' || next === '/money') mobileChat.value = true
  showSettings.value = next === '/setting'
  if (next === '/friend') { tab.value = 'people'; mobileChat.value = false }
  if (next === '/message') tab.value = 'chats'
  if (next === '/login') { showSettings.value = false; tab.value = 'chats' }
}
function navigate(path, replace = false) {
  if (!ROUTES.has(path)) path = '/login'
  if (path !== '/login' && !me.value) path = '/login'
  if (location.pathname !== path) {
    history[replace ? 'replaceState' : 'pushState']({}, '', path)
  }
  syncRoute(path)
}
function handlePopState() {
  if (!ROUTES.has(location.pathname) || (location.pathname !== '/login' && !me.value)) {
    navigate('/login',true)
  } else syncRoute(location.pathname)
}
function normalizeAddress(value) {
  try {
    const url = new URL(value.trim())
    if (url.username || url.password || url.search || url.hash || (url.protocol !== 'https:' && !(url.protocol === 'http:' && (['localhost','127.0.0.1'].includes(url.hostname) || isPrivateLANHost(url.hostname))))) throw Error('invalid')
    return url.origin
  } catch { throw Error('请输入 HTTPS 地址；HTTP 仅允许 localhost 或私有局域网 IPv4') }
}
function deviceKey() { return 'zzzmessage.device:' + server.value + '|' + (me.value?.username || '') }
function localDeviceID() {
  const key = deviceKey()
  let id = localStorage.getItem(key)
  if (!/^[a-f0-9]{32}$/.test(id || '')) {
    const bytes = new Uint8Array(16)
    crypto.getRandomValues(bytes)
    id = Array.from(bytes, b => b.toString(16).padStart(2,'0')).join('')
    localStorage.setItem(key, id)
  }
  deviceID.value = id
  return id
}
function savedTokenKey() { return `zzzmessage.token:${server.value}` }
function showError(err) { status.value = err?.message || '操作失败' }
async function request(path, options = {}) {
  const response = await fetch(server.value + path, {
    ...options,
    headers: { ...(options.body ? { 'Content-Type': 'application/json' } : {}), ...(token.value ? { Authorization: `Bearer ${token.value}` } : {}), ...options.headers },
    cache: 'no-store'
  })
  const body = await response.json().catch(() => ({}))
  if (!response.ok) throw Error(body.error || `HTTP ${response.status}`)
  return body
}
async function testServer() {
  busy.value = true; status.value = ''
  try {
    const address = normalizeAddress(editedServer.value)
    const response = await fetch(address + '/api/health', { cache: 'no-store', signal: AbortSignal.timeout(8000) })
    const data = await response.json()
    if (!response.ok || data.app !== 'ZzzMessage') throw Error('这个地址不是兼容的 ZzzMessage 后端')
    disconnect()
    server.value = address
    editedServer.value = address
    localStorage.setItem(SERVER_KEY, address)
    checkedServer.value = true
    configured.value = true
    token.value = localStorage.getItem(savedTokenKey()) || ''
    await resume()
  } catch (err) { showError(err) }
  finally { busy.value = false }
}
async function authenticate() {
  busy.value = true; status.value = ''
  try {
    const endpoint = registering.value ? '/api/auth/register' : '/api/auth/login'
    const data = await request(endpoint, {
      method: 'POST', body: JSON.stringify({ username: usernameInput.value.trim().toLowerCase(), password: password.value, ...(registering.value ? { displayName: displayName.value, inviteCode: inviteCode.value } : {}) })
    })
    password.value = ''; inviteCode.value = ''
    token.value = data.token
    localStorage.setItem(savedTokenKey(), data.token)
    me.value = data.user
    await initialize()
    navigate('/message', true)
  } catch (err) { showError(err) }
  finally { busy.value = false }
}
async function resume() {
  if (!token.value) return
  try { me.value = await request('/api/me'); await initialize() }
  catch { localStorage.removeItem(savedTokenKey()); token.value = ''; me.value = null }
}
async function initialize() {
  await refresh()
  if (!rooms.value.some(r => r.id === selectedId.value)) selectedId.value = 'lobby'
  await selectRoom(selectedId.value)
  mobileChat.value = false
  localDeviceID()
  connect()
  if (isNativeAndroid) {
    void restoreNativeRegistration()
  } else {
    void preparePush()
  }
}
async function refresh() {
  if (!me.value) return
  try {
    const [r,u] = await Promise.all([request('/api/rooms'),request('/api/users')])
    rooms.value = r; users.value = u
  } catch (err) { showError(err) }
}
async function selectRoom(id) {
  selectedId.value = id
  navigate('/message')
  mobileChat.value = true
  messages.value = []
  if (!me.value) return
  try { const selected = id; const loaded = await loadMessages(server.value, me.value.username, id); if (selectedId.value === selected) messages.value = loaded }
  catch (err) { showError(err) }
  await scrollBottom()
}
async function scrollBottom() { await nextTick(); if (chatScroll.value) chatScroll.value.scrollTop = chatScroll.value.scrollHeight }
function disconnect() {
  liveSession++
  clearTimeout(retryTimer); retryTimer = null
  if (socket) { const old = socket; socket = null; old.onclose = null; old.close() }
  connection.value = 'offline'
}
function connect() {
  if (!token.value || !me.value) return
  const generation = ++liveSession
  if (socket) { socket.onclose = null; socket.close() }
  connection.value = 'connecting'
  const ws = new WebSocket(server.value.replace(/^http/, 'ws') + '/api/ws')
  socket = ws
  ws.onopen = () => { if (generation === liveSession) ws.send(JSON.stringify({ type: 'auth', token: token.value, deviceId: localDeviceID() })) }
  ws.onmessage = async event => {
    if (generation !== liveSession) return
    let data; try { data = JSON.parse(event.data) } catch { return }
    if (data.type === 'ready') {
      connection.value = 'online'
      reconnectDelay = 1500
      requestHistorySync()
    }
    else if (data.type === 'rooms_changed') await refresh()
    else if (data.type === 'sync_status') {
      syncBusy.value = data.peers > 0
      syncStatus.value = data.peers ? ('发现 ' + data.peers + ' 台在线设备，正在合并本地历史…') : '目前没有其他在线设备，请在存有历史的设备上登录后重试。'
    }
    else if (data.type === 'sync_request') {
      void serveHistorySync(ws,generation,data)
    }
    else if (data.type === 'sync_chunk') {
      try {
        await keepMessages(server.value,me.value.username,data.messages)
        if (data.messages.some(m=>m.roomId===selectedId.value)) {
          const current=selectedId.value
          const loaded=await loadMessages(server.value,me.value.username,current)
          if (selectedId.value===current) { messages.value=loaded; await scrollBottom() }
        }
        if (generation===liveSession&&ws.readyState===WebSocket.OPEN) {
          ws.send(JSON.stringify({type:'sync_chunk_ack',requestId:data.requestId,
            deviceId:data.sourceDevice,seq:data.seq}))
        }
        syncStatus.value = '正在接收历史记录（已保存当前分片）…'
      } catch(err) {syncStatus.value='本地存储历史失败：'+err.message}
    }
    else if (data.type === 'sync_chunk_ack') {
      const key=data.requestId+':'+data.seq
      const callback=historyChunkWaiters.get(key)
      if (callback) {historyChunkWaiters.delete(key); callback()}
    }
    else if (data.type === 'sync_done') {
      syncStatus.value = '已完成来自一个设备的历史合并（按消息 ID 去重）'
      syncBusy.value=false
    }
    else if (data.type === 'gift_received') {
      status.value = '收到 '+data.from+' 赠送的 '+data.itemName
      if (routePath.value==='/gift') void openGiftShop()
    }
    else if (data.type === 'error') status.value = data.error
    else if (data.type === 'message') {
      // Persist to local IndexedDB BEFORE acknowledging server-side queue deletion.
      // Duplicate replay uses the message ID as the indexedDB key.
      try { await keepMessage(server.value, me.value.username, data) }
      catch (err) { showError(err); return }
      if (generation === liveSession && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'ack', id: data.id }))
      }
      if (data.roomId === selectedId.value && !messages.value.some(m => m.id === data.id)) {
        messages.value.push(data)
        await scrollBottom()
      }
    }
  }
  ws.onclose = () => {
    if (generation !== liveSession) return
    connection.value = 'offline'
    retryTimer = setTimeout(() => { reconnectDelay = Math.min(reconnectDelay * 1.7, 15000); connect() }, reconnectDelay)
  }
  ws.onerror = () => { if (generation === liveSession) connection.value = 'offline' }
}
function requestHistorySync() {
  if (!socket || socket.readyState !== WebSocket.OPEN || !me.value) {
    syncStatus.value = '需要先保持在线'
    return
  }
  syncBusy.value = true
  syncStatus.value = '正在请求在线设备同步历史…'
  socket.send(JSON.stringify({type:'sync_request'}))
}
async function serveHistorySync(ws,generation,request) {
  if (!request?.targetDevice || !request?.requestId || generation !== liveSession) return
  try {
    const all = await loadAllMessages(server.value,me.value.username)
    let seq = 0
    // 4 records per chunk and at most 48KB UTF-8, always waiting for the
    // target's durable IndexedDB write before sending the next chunk.
    let index = 0
    while (index < all.length && generation === liveSession && ws.readyState === WebSocket.OPEN) {
      const batch=[]
      while (index < all.length && batch.length < 4) {
        const next = all[index]
        if (batch.length && new TextEncoder().encode(JSON.stringify([...batch,next])).length > 48000) break
        batch.push(next)
        index++
      }
      const chunkIndex=seq++
      const key=request.requestId+':'+chunkIndex
      const ack=new Promise((resolve,reject)=>{
        const timer=setTimeout(()=>{
          historyChunkWaiters.delete(key)
          reject(Error('等待另一设备确认超时'))
        },20000)
        historyChunkWaiters.set(key,()=>{clearTimeout(timer);resolve()})
      })
      ws.send(JSON.stringify({type:'sync_chunk',requestId:request.requestId,
        targetDevice:request.targetDevice,seq:chunkIndex,messages:batch}))
      await ack
    }
    if (generation===liveSession && ws.readyState===WebSocket.OPEN) {
      ws.send(JSON.stringify({type:'sync_done',requestId:request.requestId,targetDevice:request.targetDevice}))
    }
  } catch(err) {
    syncStatus.value='向另一设备同步失败：'+err.message
  }
}
async function refreshWallet() {
  if (!me.value) return
  try { wallet.value=await request('/api/money') }
  catch(err) {economyStatus.value=err.message}
}
async function claimDailyCoins() {
  economyStatus.value=''
  try {
    wallet.value=await request('/api/money/daily',{method:'POST'})
    economyStatus.value='领取成功：金币 +1、银币 +5、铜币 +150'
  } catch(err) {economyStatus.value=err.message}
}
async function openGiftShop() {
  if (!me.value) return
  try {
    const [catalog,history,current]=await Promise.all([
      request('/api/gifts'), request('/api/gifts/history'),request('/api/money')
    ])
    giftCatalog.value=catalog
    giftHistory.value=history
    wallet.value=current
    if (!giftRecipient.value && users.value.length) giftRecipient.value=users.value[0].username
  } catch(err) {economyStatus.value=err.message}
}
async function purchaseGift(item) {
  if (!giftRecipient.value) {economyStatus.value='请先选择收礼人';return}
  if (giftBusy.value) return
  if (pendingGiftItemID !== item.id || pendingGiftRecipient !== giftRecipient.value) pendingGiftRequestID=''
  pendingGiftItemID=item.id
  pendingGiftRecipient=giftRecipient.value
  giftBusy.value=true
  economyStatus.value=''
  try {
    if (!pendingGiftRequestID) {
      const bytes=new Uint8Array(16)
      crypto.getRandomValues(bytes)
      pendingGiftRequestID=Array.from(bytes,b=>b.toString(16).padStart(2,'0')).join('')
    }
    const result=await request('/api/gifts/send',{
      method:'POST',
      body:JSON.stringify({recipient:giftRecipient.value,itemId:item.id,requestId:pendingGiftRequestID})
    })
    pendingGiftRequestID=''
    wallet.value=result.wallet
    giftHistory.value=await request('/api/gifts/history')
    economyStatus.value='已向 '+giftRecipient.value+' 赠送 '+item.name
  } catch(err) {
    economyStatus.value=err.message
    // Keep the same id only when a response may have been lost; a different
    // gift/recipient selection resets id with the selection controls below.
  } finally {giftBusy.value=false}
}
function giftCurrencyName(code) {
  return ({ gold:'金币',silver:'银币',copper:'铜币' })[code] || code
}
function send() {
  const text = draft.value.trim()
  if (!text || [...text].length > 4000 || !socket || socket.readyState !== WebSocket.OPEN || connection.value !== 'online') return
  socket.send(JSON.stringify({ type: 'message', roomId: selectedId.value, text }))
  draft.value = ''
}
async function direct(other) {
  status.value = ''
  try {
    const room = await request('/api/direct', { method: 'POST', body: JSON.stringify({ username: other }) })
    await refresh(); if (!rooms.value.some(r => r.id === room.id)) rooms.value.push(room)
    await selectRoom(room.id)
  } catch (err) { showError(err) }
}
async function createGroup() {
  if (!groupName.value.trim()) return
  status.value = ''
  try {
    const room = await request('/api/rooms', { method: 'POST', body: JSON.stringify({ name: groupName.value, members: groupMembers.value }) })
    newGroup.value = false; groupName.value = ''; groupMembers.value = []
    await refresh(); if (!rooms.value.some(r => r.id === room.id)) rooms.value.push(room)
    await selectRoom(room.id)
  } catch (err) { showError(err) }
}
// Prefetch both VAPID config and worker before user gesture.
// iOS requires subscribe() to begin synchronously in a tap handler.
function decodeVapid(value) {
  const padded = value.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - value.length % 4) % 4)
  return Uint8Array.from(atob(padded), c => c.charCodeAt(0))
}
function matchingPushKey(subscription) {
  const existing = subscription?.options?.applicationServerKey
  if (!existing || !pushPublicKey.value) return false
  const expected = decodeVapid(pushPublicKey.value)
  const current = new Uint8Array(existing)
  return current.length === expected.length && current.every((v, i) => v === expected[i])
}
async function preparePush() {
  pushEnabled.value = false
  pushAvailable.value = false
  pushRegistration = null
  pushStatus.value = ''
  if (!('serviceWorker' in navigator) || !('PushManager' in window) || !('Notification' in window) || !window.isSecureContext) {
    pushStatus.value = '浏览器不支持 Web Push，或页面未使用 HTTPS。'
    return
  }
  if (onIOS && !installedPWA) {
    pushStatus.value = 'iPhone 请先通过 Safari「分享 → 添加到主屏幕」，并从桌面图标打开。'
    return
  }
  try {
    const config = await request('/api/push/config')
    if (!config.enabled || !config.publicKey) {
      pushStatus.value = '服务器未配置 VAPID 密钥，请管理员查看 README。'
      return
    }
    pushPublicKey.value = config.publicKey
    pushRegistration = await Promise.race([
      navigator.serviceWorker.ready,
      new Promise((_, reject) => setTimeout(() => reject(Error('Service Worker 未就绪；请使用 HTTPS 生产版 PWA')), 10000))
    ])
    pushAvailable.value = Boolean(pushRegistration?.pushManager)
    const subscription = await pushRegistration.pushManager.getSubscription()
    if (subscription && matchingPushKey(subscription) && Notification.permission === 'granted') {
      await request('/api/push/subscription', { method: 'PUT', body: JSON.stringify(subscription.toJSON()) })
      pushEnabled.value = true
      pushStatus.value = '本设备已启用消息通知'
    } else if (subscription && !matchingPushKey(subscription)) {
      pushStatus.value = '检测到其他后端的推送订阅，请先「重置本机推送」。'
    } else {
      pushStatus.value = '点击按钮允许系统通知。'
    }
  } catch (err) { pushStatus.value = err.message || '推送初始化失败' }
}
function enablePush() {
  if (!pushRegistration || !pushPublicKey.value || pushBusy.value) {
    pushStatus.value = '尚未准备好：请检查 HTTPS、安装方式和 VAPID。'
    return
  }
  pushBusy.value = true
  pushStatus.value = '正在请求通知权限…'
  try {
    // No awaited operation before subscribe(): iOS requires a user gesture.
    const pending = pushRegistration.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: decodeVapid(pushPublicKey.value)
    })
    pending.then(async subscription => {
      await request('/api/push/subscription', { method: 'PUT', body: JSON.stringify(subscription.toJSON()) })
      pushEnabled.value = true
      pushStatus.value = '通知已开启，可发送测试通知。'
    }).catch(err => {
      pushStatus.value = '开启失败：' + (err.message || String(err)) + '。更换过服务器请先重置本机推送。'
    }).finally(() => { pushBusy.value = false })
  } catch (err) {
    pushBusy.value = false
    pushStatus.value = '开启失败：' + err.message
  }
}
async function disablePush() {
  if (!('serviceWorker' in navigator)) return
  pushBusy.value = true
  try {
    const registration = pushRegistration || await navigator.serviceWorker.getRegistration()
    const existing = await registration?.pushManager?.getSubscription()
    if (existing) {
      // Best-effort removal of server record before local unsubscribe.
      if (token.value) {
        try { await request('/api/push/subscription', { method: 'DELETE', body: JSON.stringify({ endpoint: existing.endpoint }) }) }
        catch { pushStatus.value = '旧服务器无法访问，可能暂时保留失效订阅。' }
      }
      await existing.unsubscribe()
    }
    pushEnabled.value = false
    if (!pushStatus.value.includes('旧服务器')) pushStatus.value = '本设备通知已关闭'
  } catch (err) { pushStatus.value = '关闭推送失败：' + err.message }
  finally { pushBusy.value = false }
}
async function sendTestPush() {
  pushStatus.value = ''
  try {
    const reply = await request('/api/push/test', { method: 'POST' })
    pushStatus.value = reply.queued ? '测试推送已入队，请检查系统通知、网络及专注模式。' : '当前没有可用的推送订阅。'
  } catch (err) { pushStatus.value = err.message }
}
async function registerNativeDevice(device) {
  if (!me.value || !device?.provider || !device?.token) return
  nativePushBusy.value = true
  try {
    await request('/api/native-push/device', {
      method: 'PUT',
      body: JSON.stringify({ provider: device.provider, token: device.token })
    })
    nativePushBound.value = true
    nativePushStatus.value = '已绑定厂商推送设备：' + device.provider
  } catch (err) { nativePushStatus.value = '绑定失败：' + err.message }
  finally { nativePushBusy.value = false }
}
function onNativePushEvent(event) { void registerNativeDevice(event.detail) }
function onNativePushError(event) { nativePushStatus.value = String(event.detail || '厂商初始化失败') }
async function restoreNativeRegistration() {
  if (!isNativeAndroid || !me.value) return
  try {
    const data = JSON.parse(window.NativePushBridge.registration() || '{}')
    if (data.provider && data.token) await registerNativeDevice(data)
    else nativePushStatus.value = '首次使用请点击「启用小米 / vivo 系统通知」。'
  } catch (err) { nativePushStatus.value = err.message || '读取设备推送状态失败' }
}
async function enableNativePush() {
  if (!nativeSupported || nativePushBusy.value) return
  nativePushBusy.value = true
  try {
    const available = await request('/api/native-push/config')
    const vendor = ['xiaomi', 'redmi'].includes(nativeManufacturer) ? 'xiaomi' : 'vivo'
    if (!available[vendor]) {
      nativePushStatus.value = '管理员尚未在 Go 服务端配置该厂商的推送密钥'
      return
    }
    nativePushStatus.value = '正在初始化系统推送并请求通知授权…'
    // Registration callback delivers token through zzz:native-push event.
    window.NativePushBridge.enable()
  } catch (err) { nativePushStatus.value = err.message }
  finally { nativePushBusy.value = false }
}
async function disableNativePush() {
  if (!isNativeAndroid) return
  nativePushBusy.value = true
  try {
    const device = JSON.parse(window.NativePushBridge.registration() || '{}')
    if (device.provider && device.token && token.value) {
      await request('/api/native-push/device', {
        method: 'DELETE', body: JSON.stringify({ provider: device.provider, token: device.token })
      })
    }
    window.NativePushBridge.disable()
    nativePushBound.value = false
    nativePushStatus.value = '本机已取消厂商系统通知'
  } catch (err) { nativePushStatus.value = '取消通知失败：' + err.message }
  finally { nativePushBusy.value = false }
}
async function testNativePush() {
  nativePushStatus.value = ''
  try {
    const res = await request('/api/native-push/test', { method: 'POST' })
    nativePushStatus.value = res.queued ? '已请求厂商发送测试通知（实际送达需真机确认）' : '尚无绑定设备'
  } catch (err) { nativePushStatus.value = err.message }
}
async function clearLocal() {
  if (!me.value || !confirm('仅清除本设备当前会话的聊天记录，确认继续？')) return
  try { await clearRoom(server.value, me.value.username, selectedId.value); messages.value = [] } catch (err) { showError(err) }
}
async function logout() {
  if (isNativeAndroid) await disableNativePush()
  else await disablePush()
  disconnect()
  localStorage.removeItem(savedTokenKey())
  token.value = ''; me.value = null; rooms.value = []; users.value = []; messages.value = []
  showSettings.value = false; status.value = ''
  navigate('/login',true)
}
async function changeServer() {
  await logout()
  configured.value = false
  checkedServer.value = false
  editedServer.value = server.value
}
function formatTime(v) { try { return new Date(v).toLocaleTimeString('zh-CN', { hour:'2-digit', minute:'2-digit' }) } catch { return '' } }
onMounted(async () => {
  window.addEventListener('popstate', handlePopState)
  if (location.pathname === '/' || !ROUTES.has(location.pathname)) navigate('/login', true)
  const wantedPath = routePath.value
  window.addEventListener('zzz:native-push', onNativePushEvent)
  window.addEventListener('zzz:native-push-error', onNativePushError)
  token.value = localStorage.getItem(savedTokenKey()) || ''
  try {
    const health = await request('/api/health')
    if (health.app === 'ZzzMessage') {
      checkedServer.value = true
      configured.value = true
      await resume()
    }
  } catch { configured.value = false }
  if (me.value) navigate(wantedPath === '/login' ? '/message' : wantedPath, true)
  else navigate('/login',true)
})
onBeforeUnmount(() => {
  window.removeEventListener('popstate', handlePopState)
  window.removeEventListener('zzz:native-push', onNativePushEvent)
  window.removeEventListener('zzz:native-push-error', onNativePushError)
  disconnect()
})
</script>

<template>
  <main class="shell">
    <section v-if="!configured" class="welcome card">
      <div class="brand-icon">Z</div>
      <h1>连接你的私人服务器</h1>
      <p class="muted">使用自己部署的 Go 后端。服务器之间账号与消息互相独立。</p>
      <label class="field-label">后端地址</label>
      <input v-model="editedServer" placeholder="https://chat.example.com" @keyup.enter="testServer" />
      <p class="hint">需支持 /api/health。公网请使用 HTTPS；局域网私有 IP 可使用 HTTP（此时无法启用 Web Push）。</p>
      <button class="primary" :disabled="busy" @click="testServer">{{ busy ? '正在检查...' : '验证并固定服务器' }}</button>
      <p v-if="status" class="error" role="alert">{{ status }}</p>
    </section>

    <section v-else-if="!me" class="welcome card">
      <div class="brand-icon">Z</div>
      <h1>{{ registering ? '邀请注册' : '欢迎回来' }}</h1>
      <p class="muted">服务器：{{ serverName }}</p>
      <div class="stack">
        <label class="field-label">用户名（英文小写 / 数字 / 下划线）</label>
        <input v-model="usernameInput" maxlength="24" autocomplete="username" placeholder="alice" />
        <template v-if="registering"><label class="field-label">显示昵称</label><input v-model="displayName" maxlength="40" placeholder="小明" /></template>
        <label class="field-label">密码</label>
        <input v-model="password" type="password" autocomplete="current-password" placeholder="至少 8 位" @keyup.enter="authenticate" />
        <template v-if="registering"><label class="field-label">邀请代码</label><input v-model="inviteCode" type="password" placeholder="向服务器管理员索取" /></template>
        <button class="primary" :disabled="busy" @click="authenticate">{{ busy ? '请稍候...' : registering ? '创建账号' : '登录' }}</button>
      </div>
      <p v-if="status" class="error" role="alert">{{ status }}</p>
      <button class="link" @click="registering = !registering; status = ''">{{ registering ? '已有账号？登录' : '持有邀请码？注册账号' }}</button>
      <button class="link subtle" @click="changeServer">更换服务器</button>
    </section>

    <section v-else class="app card" :class="{ 'mobile-chat': mobileChat }">
      <aside class="sidebar">
        <header class="sidebar-head">
          <div class="brand"><span class="small-icon">Z</span><div><strong>ZzzMessage</strong><small>{{ serverName }}</small></div></div>
          <button class="icon-button" aria-label="设置" @click="navigate(showSettings ? '/message' : '/setting'); newGroup = false">⚙</button>
        </header>
        <div class="nav-tabs">
          <button :class="{ chosen: tab === 'chats' }" @click="navigate('/message')">会话</button>
          <button :class="{ chosen: tab === 'people' }" @click="navigate('/friend'); refresh()">联系人</button>
          <button :class="{ chosen: routePath === '/gift' }" @click="navigate('/gift')">礼物</button>
          <button :class="{ chosen: routePath === '/money' }" @click="navigate('/money')">钱包</button>
        </div>
        <div v-if="showSettings" class="sidebar-body">
          <h3>设置</h3>
          <p class="muted">账号：{{ me.displayName }} (@{{ me.username }})</p>
          <p class="muted">后端：{{ server }}</p>
          <h3>多设备本地历史同步</h3>
          <p class="hint">需要另一台存有聊天记录的同账号设备在线；历史只在设备之间中转，不长期保存在服务器。</p>
          <p class="hint" role="status">{{ syncStatus }}</p>
          <button class="secondary" :disabled="connection !== 'online' || syncBusy" @click="requestHistorySync">从其他在线设备同步历史</button>
          <p class="hint">聊天正文首先存入服务端离线待投递队列（最多保留14天/每账号500条），客户端确认保存到 IndexedDB 后删除该账号待投递副本。当前没有端到端或本地静态加密。</p>
          <h3>{{ isNativeAndroid ? 'Android 原生系统推送' : 'Web / iPhone PWA 系统通知' }}</h3>
          <template v-if="isNativeAndroid">
            <p class="hint">厂商识别：{{ nativeManufacturer }}。首批支持 Xiaomi/Redmi、vivo/iQOO；仅在你主动同意后初始化 SDK。</p>
            <p class="hint">推送只显示通用「收到新消息」，不包含聊天正文。</p>
            <p class="hint" role="status">{{ nativePushStatus }}</p>
            <button v-if="nativeSupported" class="primary" :disabled="nativePushBusy" @click="enableNativePush">启用小米 / vivo 系统通知</button>
            <button v-if="nativePushBound" class="secondary" :disabled="nativePushBusy" @click="testNativePush">发送厂商测试通知</button>
            <button v-if="nativeSupported" class="secondary" :disabled="nativePushBusy" @click="disableNativePush">关闭厂商推送</button>
            <p v-if="!nativeSupported" class="hint">此 Android 设备暂不在首批厂商推送支持范围。</p>
          </template>
          <template v-else>
          <p class="hint">系统通知只显示「收到新消息」，不包含正文。消息离线期间会在恢复连接后补发。</p>
          <p v-if="onIOS && !installedPWA" class="hint">iPhone：Safari 访问 HTTPS 网站 → 分享 → 添加到主屏幕 → 从桌面图标启动 → 登录后开启通知。需要 iOS 16.4+。</p>
          <p v-else class="hint">Android 和桌面浏览器：使用 HTTPS 站点或 PWA，允许网站通知。实际送达取决于系统和浏览器。</p>
          <p class="hint" role="status">{{ pushStatus }}</p>
          <button v-if="!pushEnabled" class="primary" :disabled="pushBusy || !pushAvailable" @click="enablePush">开启本设备通知</button>
          <button v-else class="secondary" :disabled="pushBusy" @click="sendTestPush">发送测试通知</button>
          <button class="secondary" :disabled="pushBusy" @click="disablePush">{{ pushEnabled ? '关闭本设备通知' : '重置本机推送' }}</button>
          </template>
          <button class="secondary" @click="clearLocal">清空当前会话本地记录</button>
          <button class="secondary" @click="changeServer">切换服务器</button>
          <button class="danger" @click="logout">退出登录</button>
          <button class="link" @click="navigate('/message')">返回聊天</button>
        </div>
        <div v-else-if="newGroup" class="sidebar-body">
          <h3>新建群聊</h3>
          <input v-model="groupName" placeholder="群聊名称" maxlength="50" />
          <label class="field-label">选择群成员</label>
          <label v-for="u in users" :key="u.username" class="member"><input v-model="groupMembers" type="checkbox" :value="u.username" /> {{ u.displayName }} <small>@{{ u.username }}</small></label>
          <p class="hint">已选择 {{ selectedUsers.length }} 人；最多 30 人。</p>
          <button class="primary" @click="createGroup">创建群聊</button>
          <button class="link" @click="newGroup = false">取消</button>
        </div>
        <div v-else-if="tab === 'people'" class="room-list">
          <div class="list-title">成员 <button class="link" @click="refresh">刷新</button></div>
          <button v-for="u in users" :key="u.username" class="room-item" @click="direct(u.username)"><span class="avatar">{{ u.displayName[0] }}</span><span class="room-copy"><strong>{{ u.displayName }}</strong><small>@{{ u.username }} · 发起私聊</small></span></button>
          <p v-if="!users.length" class="empty-note">目前还没有其他成员</p>
        </div>
        <div v-else class="room-list">
          <div class="list-title">聊天会话 <button class="link" @click="newGroup = true; refresh()">+ 新建群聊</button></div>
          <button v-for="r in rooms" :key="r.id" class="room-item" :class="{ active: selectedId === r.id }" @click="selectRoom(r.id)"><span class="avatar" :class="{ group: r.type !== 'direct' }">{{ r.type === 'direct' ? '◉' : '#' }}</span><span class="room-copy"><strong>{{ roomLabel(r) }}</strong><small>{{ r.type === 'direct' ? '私人会话' : r.type === 'lobby' ? '同一服务器成员' : `${r.members.length} 位成员` }}</small></span></button>
        </div>
        <footer class="sidebar-foot"><span :class="['dot', connection]"></span>{{ connection === 'online' ? '实时连接正常' : connection === 'connecting' ? '正在连接' : '连接中断，将自动重试' }}</footer>
      </aside>
      <section v-if="routePath === '/money'" class="economy-pane">
        <header class="economy-header">
          <button class="mobile-back" @click="mobileChat=false; navigate('/message')">‹ 返回</button>
          <h2>我的钱包</h2>
          <p>金币、银币、铜币均为站内虚拟积分，不能提现或兑换现实货币。</p>
        </header>
        <div class="coin-grid">
          <article class="coin-card"><span>金币</span><strong>{{ wallet.gold }}</strong><small>Gold</small></article>
          <article class="coin-card"><span>银币</span><strong>{{ wallet.silver }}</strong><small>Silver</small></article>
          <article class="coin-card"><span>铜币</span><strong>{{ wallet.copper }}</strong><small>Copper</small></article>
        </div>
        <div class="economy-actions">
          <h3>每日奖励</h3>
          <p>每日按 UTC 日期领取金币 1、银币 5、铜币 150，每天仅能领取一次。</p>
          <button class="primary" @click="claimDailyCoins">领取今日奖励</button>
          <button class="secondary" @click="navigate('/gift')">前往礼物商店</button>
        </div>
        <p class="hint" role="status">{{ economyStatus }}</p>
      </section>
      <section v-else-if="routePath === '/gift'" class="economy-pane">
        <header class="economy-header">
          <button class="mobile-back" @click="mobileChat=false; navigate('/message')">‹ 返回</button>
          <h2>礼物商店</h2>
          <p>用虚拟金币、银币或铜币购买礼物，送给其他已注册的用户。</p>
        </header>
        <div class="economy-actions">
          <label class="field-label">选择收礼人</label>
          <select v-model="giftRecipient" @change="pendingGiftRequestID=''">
            <option disabled value="">请选择用户</option>
            <option v-for="u in users" :key="u.username" :value="u.username">{{ u.displayName }} (@{{ u.username }})</option>
          </select>
          <div class="wallet-strip">余额：金币 {{ wallet.gold }} · 银币 {{ wallet.silver }} · 铜币 {{ wallet.copper }}</div>
        </div>
        <div class="gift-grid">
          <article v-for="gift in giftCatalog" :key="gift.id" class="gift-card">
            <div class="gift-icon">{{ gift.icon }}</div>
            <strong>{{ gift.name }}</strong>
            <small>{{ gift.description }}</small>
            <div>{{ gift.price }} {{ giftCurrencyName(gift.currency) }}</div>
            <button class="primary" :disabled="giftBusy || !giftRecipient" @click="purchaseGift(gift)">购买并赠送</button>
          </article>
        </div>
        <p class="hint" role="status">{{ economyStatus }}</p>
        <div class="economy-actions">
          <h3>近期礼物记录</h3>
          <p v-for="g in giftHistory" :key="g.requestId">{{ g.sender === me.username ? '送给 '+g.recipient : '收到 '+g.sender+' 的' }} · {{ g.itemName }} · {{ formatTime(g.createdAt) }}</p>
          <p v-if="!giftHistory.length">暂无礼物记录</p>
        </div>
      </section>
      <div v-else class="chat-pane">
        <header class="chat-head"><button class="mobile-back" type="button" aria-label="返回会话" @click="mobileChat = false">‹ 返回</button><div><strong>{{ activeRoom ? roomLabel(activeRoom) : '选择聊天' }}</strong><small>{{ activeRoom?.type === 'direct' ? '一对一私聊' : '离线自动补发 · 本地保存历史' }}</small></div><button class="icon-button" title="刷新会话" @click="refresh">⟳</button></header>
        <div ref="chatScroll" class="timeline">
          <div class="info-banner">消息会先由服务器保存到待投递队列，恢复连接后补发并留在本机；服务器收到收妥确认后删除待投递副本。当前不是端到端加密。</div>
          <p v-if="sortedMessages.length === 0" class="empty-chat">还没有本地聊天记录。发送第一条消息吧。</p>
          <div v-for="m in sortedMessages" :key="m.id" class="message" :class="{ mine: m.sender === me.username }"><div class="bubble"><small>{{ m.sender }} · {{ formatTime(m.createdAt) }}</small><p>{{ m.text }}</p></div></div>
        </div>
        <form class="composer" @submit.prevent="send"><input v-model="draft" maxlength="4000" :disabled="connection !== 'online'" :placeholder="connection === 'online' ? '输入消息，按 Enter 发送' : '等待服务器连接...'" /><button type="submit" class="primary" :disabled="!draft.trim() || connection !== 'online'">发送</button></form>
        <div v-if="status" class="toast" role="alert">{{ status }} <button @click="status = ''" aria-label="关闭提示">×</button></div>
      </div>
    </section>
  </main>
</template>

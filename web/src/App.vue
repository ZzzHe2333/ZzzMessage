<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { keepMessage, loadMessages, clearRoom } from './localMessages'

const SERVER_KEY = 'zzzmessage.server'
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

function normalizeAddress(value) {
  try {
    const url = new URL(value.trim())
    if (url.username || url.password || url.search || url.hash || (url.protocol !== 'https:' && !(url.protocol === 'http:' && ['localhost','127.0.0.1'].includes(url.hostname)))) throw Error('invalid')
    return url.origin
  } catch { throw Error('请输入 HTTPS 服务器地址（本地开发允许 localhost HTTP）') }
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
  connect()
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
  tab.value = 'chats'
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
  ws.onopen = () => { if (generation === liveSession) ws.send(JSON.stringify({ type: 'auth', token: token.value })) }
  ws.onmessage = async event => {
    if (generation !== liveSession) return
    let data; try { data = JSON.parse(event.data) } catch { return }
    if (data.type === 'ready') { connection.value = 'online'; reconnectDelay = 1500 }
    else if (data.type === 'rooms_changed') await refresh()
    else if (data.type === 'error') status.value = data.error
    else if (data.type === 'message') {
      // Local IndexedDB is the only persistence layer for message bodies.
      try { await keepMessage(server.value, me.value.username, data) } catch (err) { showError(err) }
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
async function clearLocal() {
  if (!me.value || !confirm('仅清除本设备当前会话的聊天记录，确认继续？')) return
  try { await clearRoom(server.value, me.value.username, selectedId.value); messages.value = [] } catch (err) { showError(err) }
}
function logout() {
  disconnect()
  localStorage.removeItem(savedTokenKey())
  token.value = ''; me.value = null; rooms.value = []; users.value = []; messages.value = []
  showSettings.value = false; status.value = ''
}
function changeServer() { logout(); configured.value = false; checkedServer.value = false; editedServer.value = server.value }
function formatTime(v) { try { return new Date(v).toLocaleTimeString('zh-CN', { hour:'2-digit', minute:'2-digit' }) } catch { return '' } }
onMounted(async () => {
  token.value = localStorage.getItem(savedTokenKey()) || ''
  try { const health = await request('/api/health'); if (health.app === 'ZzzMessage') { checkedServer.value = true; configured.value = true; await resume() } } catch { configured.value = false }
})
onBeforeUnmount(disconnect)
</script>

<template>
  <main class="shell">
    <section v-if="!configured" class="welcome card">
      <div class="brand-icon">Z</div>
      <h1>连接你的私人服务器</h1>
      <p class="muted">使用自己部署的 Go 后端。服务器之间账号与消息互相独立。</p>
      <label class="field-label">后端地址</label>
      <input v-model="editedServer" placeholder="https://chat.example.com" @keyup.enter="testServer" />
      <p class="hint">必须支持 HTTPS 和 /api/health；本机 localhost 调试可使用 HTTP。</p>
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

    <section v-else class="app card">
      <aside class="sidebar">
        <header class="sidebar-head">
          <div class="brand"><span class="small-icon">Z</span><div><strong>ZzzMessage</strong><small>{{ serverName }}</small></div></div>
          <button class="icon-button" aria-label="设置" @click="showSettings = !showSettings; newGroup = false">⚙</button>
        </header>
        <div class="nav-tabs">
          <button :class="{ chosen: tab === 'chats' }" @click="tab = 'chats'">会话</button>
          <button :class="{ chosen: tab === 'people' }" @click="tab = 'people'; refresh()">联系人</button>
        </div>
        <div v-if="showSettings" class="sidebar-body">
          <h3>设置</h3>
          <p class="muted">账号：{{ me.displayName }} (@{{ me.username }})</p>
          <p class="muted">后端：{{ server }}</p>
          <p class="hint">聊天正文只保存在各客户端的 IndexedDB；目前未做端到端或本地静态加密。</p>
          <button class="secondary" @click="clearLocal">清空当前会话本地记录</button>
          <button class="secondary" @click="changeServer">切换服务器</button>
          <button class="danger" @click="logout">退出登录</button>
          <button class="link" @click="showSettings = false">返回聊天</button>
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
      <div class="chat-pane">
        <header class="chat-head"><div><strong>{{ activeRoom ? roomLabel(activeRoom) : '选择聊天' }}</strong><small>{{ activeRoom?.type === 'direct' ? '一对一私聊' : '实时消息 · 不保存云端历史' }}</small></div><button class="icon-button" title="刷新会话" @click="refresh">⟳</button></header>
        <div ref="chatScroll" class="timeline">
          <div class="info-banner">这里只显示本设备收到的消息。本设备离线期间的消息不会补发；这不是端到端加密聊天。</div>
          <p v-if="sortedMessages.length === 0" class="empty-chat">还没有本地聊天记录。发送第一条消息吧。</p>
          <div v-for="m in sortedMessages" :key="m.id" class="message" :class="{ mine: m.sender === me.username }"><div class="bubble"><small>{{ m.sender }} · {{ formatTime(m.createdAt) }}</small><p>{{ m.text }}</p></div></div>
        </div>
        <form class="composer" @submit.prevent="send"><input v-model="draft" maxlength="4000" :disabled="connection !== 'online'" :placeholder="connection === 'online' ? '输入消息，按 Enter 发送' : '等待服务器连接...'" /><button type="submit" class="primary" :disabled="!draft.trim() || connection !== 'online'">发送</button></form>
        <div v-if="status" class="toast" role="alert">{{ status }} <button @click="status = ''" aria-label="关闭提示">×</button></div>
      </div>
    </section>
  </main>
</template>

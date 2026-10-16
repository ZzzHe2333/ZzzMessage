<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { keepMessage, keepMessages, loadMessages, loadAllMessages, clearRoom, deleteLocalMessage } from './localMessages'
import { scopeKey, conversationSummaries, sortConversations, searchLocalMessages, previewOf, summarizeReactions } from './chatUiState'

const SERVER_KEY = 'zzzmessage.server'
const ROUTES = new Set(['/login','/setting','/message','/friend','/gift','/money','/api-key'])
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
const apiConnected = ref(false)
const pushPreviewEnabled = ref(true)
const status = ref('')
const busy = ref(false)
const tab = ref('chats')
const groupName = ref('')
const chatSearch = ref('')
const mobileFilter = ref('all')
const mobileFilterOptions = [
  {value:'all',label:'全部'},
  {value:'unread',label:'未读'},
  {value:'groups',label:'群组'}
]
const mobileBuildVersion = '2026.10.16-mobile.1'
const mobileUpdateStatus = ref('')

const allLocalMessages = ref([])
const chatPrefs = ref({})
const replyTo = ref(null)
const reactionChoices = ['👍','❤️','😂','😮','😢','🎉']
const reactionBusy = ref(false)
const messageActions = ref('')
const messageSearchOpen = ref(false)
const messageSearch = ref('')
const highlightedId = ref('')
const forwardSource = ref(null)
const forwardTarget = ref('')
const sending = ref(false)
const menuOpen = ref(false)
const infoOpen = ref(false)
const emojiOpen = ref(false)
const uploadBusy = ref(false)
const mediaPreview = ref(null)
const dragOverChat = ref(false)
const fileInput = ref(null)
const recentEmoji = ['😀','😁','😂','🥰','😍','😘','😎','😭','🙏','👍','❤️','🎉','🔥','🌷','☕','🤝','💡','✅','😴','🥳','🌈','💬','👌','👀']

const groupMembers = ref([])
const newGroup = ref(false)
const showSettings = ref(false)
const apiKeys = ref([])
const keyLabel = ref('')
const keyRecipients = ref([])
const keyValidDays = ref(90)
const keyNewSecret = ref('')
const keyNewDetails = ref(null)
const keyBusy = ref(false)
const keyNotice = ref('')
const keyError = ref('')
const keyEndpoint = computed(()=>server.value+'/api/external/send')
const allowSecureKeys = computed(()=>location.protocol==='https:' || ['localhost','127.0.0.1'].includes(location.hostname))
const keyExample = computed(()=>[
  "POST "+keyEndpoint.value,
  "Authorization: Bearer YOUR_API_KEY",
  "Content-Type: application/json",
  "",
  JSON.stringify({recipient:keyRecipients.value[0] || 'recipient_username',text:'自动通知：任务执行成功'},null,2)
].join('\n'))
async function refreshAPIKeys(){
  if(!me.value)return
  keyError.value=''
  try {
    const result=await request('/api/keys')
    apiKeys.value=Array.isArray(result.keys)?result.keys:[]
    if(!users.value.length)await refresh()
  } catch(err){keyError.value=err?.message||String(err)}
}
async function createAPIKey(){
  if(keyBusy.value)return
  keyError.value='';keyNotice.value=''
  if(!keyLabel.value.trim()||!keyRecipients.value.length){
    keyError.value='请填写名称，并至少勾选一个收件人'
    return
  }
  if(!allowSecureKeys.value){
    keyError.value='创建密钥请使用 HTTPS 或本机 localhost，避免明文 Key 在局域网中泄露'
    return
  }
  keyBusy.value=true
  try {
    const result=await request('/api/keys',{method:'POST',body:JSON.stringify({
      label:keyLabel.value.trim(),recipients:keyRecipients.value,validDays:Number(keyValidDays.value)
    })})
    keyNewDetails.value=result.key
    keyNewSecret.value=result.secret
    keyLabel.value=''
    keyRecipients.value=[]
    await refreshAPIKeys()
    keyNotice.value='Key 已创建。请立即复制并安全保存，关闭后不能再次查看。'
  }catch(err){keyError.value=err?.message||String(err)}
  finally{keyBusy.value=false}
}
async function copyKeyEndpoint(){
  try{await navigator.clipboard.writeText(keyEndpoint.value);keyNotice.value='已复制 API 地址'}
  catch(error){keyError.value='复制地址失败：'+(error?.message||error)}
}
async function copyNewAPIKey(){
  if(!keyNewSecret.value)return
  try{await navigator.clipboard.writeText(keyNewSecret.value);keyNotice.value='已复制 Key。请保存在可信的密钥管理器中。'}
  catch(err){keyError.value='复制失败，请手动选中完整 Key：'+(err?.message||err)}
}
async function revokeAPIKey(key){
  if(keyBusy.value||!window.confirm('确定撤销“'+key.label+'”？使用该 Key 的自动通知将立即停止。'))return
  keyBusy.value=true;keyError.value='';keyNotice.value=''
  try{
    await request('/api/keys/'+encodeURIComponent(key.id),{method:'DELETE'})
    await refreshAPIKeys()
    if(keyNewDetails.value?.id===key.id){keyNewSecret.value='';keyNewDetails.value=null}
    keyNotice.value='已撤销 '+key.label
  }catch(err){keyError.value=err?.message||String(err)}
  finally{keyBusy.value=false}
}
function dismissNewAPIKey(){
  keyNewSecret.value=''
  keyNewDetails.value=null
}

const mobileChat = ref(false)
const deviceID = ref('')
const syncStatus = ref('未请求同步')
const syncBusy = ref(false)
const historyChunkWaiters = new Map()
const wallet = ref({ gold:0, silver:0, copper:0, lastDaily:'', lastReward:'' })
const dailyNotice = ref('')
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
let pushExistingSubscription = null
let socket = null
let retryTimer = null
let pollingTimer = null
let pollingBusy = false
let roomLoadSequence = 0
let reconnectDelay = 1500
let liveSession = 0

const activeRoom = computed(() => rooms.value.find(r => r.id === selectedId.value))
const roomLabel = r => {
  if (r.type !== 'direct') return r.name
  const username = r.members.find(x => x !== me.value?.username)
  return users.value.find(u => u.username === username)?.displayName || username || '私聊'
}
const roomAvatar = r => (r?.type === 'direct' ? roomLabel(r).slice(0,1) : r?.type === 'group' ? '👥' : '💬')
const roomSummaries = computed(() => conversationSummaries(allLocalMessages.value,me.value?.username||'',chatPrefs.value))
const filteredRooms = computed(() => sortConversations(
  rooms.value.filter(r => mobileFilter.value==='groups' ? r.type==='group' :
    mobileFilter.value==='unread' ? (roomSummaries.value[r.id]?.unread||0)>0 : true),
  roomSummaries.value,chatPrefs.value,chatSearch.value,roomLabel
))
const searchedMessages = computed(() => searchLocalMessages(
  allLocalMessages.value,chatSearch.value,rooms.value.map(r=>r.id),25))
const inRoomSearch = computed(() => searchLocalMessages(
  allLocalMessages.value.filter(m=>m.roomId===selectedId.value),messageSearch.value,[selectedId.value],80))
const roomUnread = id => roomSummaries.value[id]?.unread || 0
const lastInRoom = id => roomSummaries.value[id]?.latest || null
const previewRoom = id => {
  const localDraft = id===selectedId.value ? draft.value : readRoomDraft(id)
  if(localDraft.trim()) return '草稿：'+localDraft.replace(/\s+/g,' ').slice(0,65)
  return previewOf(lastInRoom(id),me.value?.username)
}
const roomTime = id => lastInRoom(id) ? formatTime(lastInRoom(id).createdAt) : ''
const roomPinned = id => !!chatPrefs.value[id]?.pinned
const roomMuted = id => !!chatPrefs.value[id]?.muted
const replyMessage = id => allLocalMessages.value.find(m=>m.roomId===selectedId.value && m.id===id)

const filteredPeople = computed(() => users.value.filter(u =>
  (u.displayName + ' ' + u.username).toLowerCase().includes(chatSearch.value.trim().toLowerCase())
))
const activeSubtitle = computed(() => activeRoom.value?.type==='group'
  ? activeRoom.value.members.length + ' 位成员'
  : activeRoom.value?.type==='direct' ? '私人对话' : '所有服务器成员')
const displayTitle = computed(() => activeRoom.value ? roomLabel(activeRoom.value) + ' · ZzzMessage' : 'ZzzMessage')
watch([displayTitle, routePath], () => {
  document.title = routePath.value === '/message' ? displayTitle.value : 'ZzzMessage · ' + routePath.value.slice(1)
}, { immediate:true })
async function checkMobileUpdate() {
  mobileUpdateStatus.value='正在检查前端更新…'
  try {
    if (!('serviceWorker' in navigator)) {
      mobileUpdateStatus.value='当前浏览器未使用 PWA 更新服务。请直接刷新网页。'
      return
    }
    const registration=await navigator.serviceWorker.getRegistration()
    if (!registration) {
      mobileUpdateStatus.value='尚未安装 PWA 缓存，刷新网页即可获取新页面。'
      return
    }
    await registration.update()
    mobileUpdateStatus.value='已检查更新。如发现新版本将自动切换并刷新页面；服务器仍须部署新 web 文件。'
  } catch(error) {
    mobileUpdateStatus.value='检查更新失败：'+(error?.message||error)
  }
}
function goMenu(target) {
  menuOpen.value=false
  if (target==='/friend') void refresh()
  navigate(target)
}
function openNewGroup() {
  menuOpen.value=false
  navigate('/message')
  showSettings.value=false
  newGroup.value=true
  void refresh()
}
function prefStorageKey() {
  return me.value ? scopeKey(server.value,me.value.username,'prefs') : ''
}
function readRoomDraft(roomId) {
  if(!me.value || !roomId)return ''
  try{return localStorage.getItem(scopeKey(server.value,me.value.username,'draft:'+roomId))||''}catch{return ''}
}
function persistRoomDraft(roomId,content){
  if(!me.value||!roomId)return
  try{
    const key=scopeKey(server.value,me.value.username,'draft:'+roomId)
    if(content)localStorage.setItem(key,content)
    else localStorage.removeItem(key)
  }catch{}
}
watch(draft,value=>{
  if(me.value)persistRoomDraft(selectedId.value,value)
},{flush:'sync'})
function restoreChatPreferences(){
  try{
    const saved=JSON.parse(localStorage.getItem(prefStorageKey())||'{}')
    chatPrefs.value=saved&&typeof saved==='object'&&!Array.isArray(saved)?saved:{}
  }catch{chatPrefs.value={}}
}
function setRoomPreference(roomId,changes){
  if(!me.value||!roomId)return
  chatPrefs.value={...chatPrefs.value,[roomId]:{...(chatPrefs.value[roomId]||{}),...changes}}
  try{localStorage.setItem(prefStorageKey(),JSON.stringify(chatPrefs.value))}catch{}
}
function togglePinned(roomId){
  setRoomPreference(roomId,{pinned:!roomPinned(roomId)})
}
async function toggleMuted(roomId){
  try{
    const desired=!roomMuted(roomId)
    const result=await request('/api/rooms/mutes',{
      method:'PUT',body:JSON.stringify({roomId,muted:desired})
    })
    setRoomPreference(roomId,{muted:!!result.muted})
    status.value=result.muted?'此会话的系统通知已静音（当前账号全部设备）':'此会话的系统通知已恢复'
  }catch(err){showError(err)}
}
async function refreshServerMutes(){
  if(!me.value)return
  try{
    const reply=await request('/api/rooms/mutes')
    const updated={...chatPrefs.value}
    for(const room of rooms.value){
      updated[room.id]={...(updated[room.id]||{}),muted:!!reply.mutedRooms?.[room.id]}
    }
    chatPrefs.value=updated
    localStorage.setItem(prefStorageKey(),JSON.stringify(updated))
  }catch{
    // Older backends don't have this route. Keep local state for migration.
  }
}
function chatIsVisible(){
  return routePath.value==='/message' &&
    document.visibilityState==='visible' &&
    (window.innerWidth>680||mobileChat.value)
}
function markRoomRead(roomId){
  if(!roomId||!chatIsVisible())return
  const latest=roomSummaries.value[roomId]?.latest?.createdAt
  if(!latest)return
  if((chatPrefs.value[roomId]?.lastRead||'')>=latest)return
  setRoomPreference(roomId,{lastRead:latest})
}
async function rebuildHistoryIndex(){
  if(!me.value){allLocalMessages.value=[];return}
  try {allLocalMessages.value=await loadAllMessages(server.value,me.value.username)}
  catch(err){showError(Error('本机历史索引加载失败：'+(err?.message||err)))}
}
function localMessageAdded(message){
  if(!allLocalMessages.value.some(m=>m.id===message.id && m.roomId===message.roomId))
    allLocalMessages.value=[...allLocalMessages.value,message]
}
async function jumpToMessage(message){
  if(!message)return
  if(selectedId.value!==message.roomId)await selectRoom(message.roomId)
  chatSearch.value=''
  messageSearchOpen.value=false
  await nextTick()
  const match=Array.from(chatScroll.value?.querySelectorAll('[data-message-id]')||[])
    .find(el=>el.dataset.messageId===message.id)
  if(match){
    match.scrollIntoView({behavior:'smooth',block:'center'})
    highlightedId.value=message.id
  }
}
function openReply(message){
  replyTo.value=message
  messageActions.value=''
  emojiOpen.value=false
  nextTick(()=>document.querySelector('.tg-composer textarea')?.focus())
}
async function reactTo(message,emoji){
  if(reactionBusy.value || !message?.id || !me.value)return
  reactionBusy.value=true
  try{
    const event=await request('/api/messages/react',{
      method:'POST',body:JSON.stringify({
        roomId:message.roomId,messageId:message.id,emoji
      })
    })
    await acceptMessage(event)
    messageActions.value=''
  }catch(err){showError(err)}
  finally{reactionBusy.value=false}
}
async function copyMessage(message){
  try{await navigator.clipboard.writeText(message.text||message.attachment?.name||'')
    status.value='已复制消息文字'
  }catch(err){status.value='复制失败：'+(err?.message||err)}
  messageActions.value=''
}
function beginForward(message){
  if(message.attachment){status.value='附件暂不支持一键转发，请先下载后重新上传';return}
  forwardSource.value=message
  forwardTarget.value=selectedId.value
  messageActions.value=''
}
async function submitForward(){
  if(!forwardSource.value||!forwardTarget.value||sending.value)return
  sending.value=true
  try{
    const quote='↪ 转发消息\n'+forwardSource.value.text
    const msg=await request('/api/messages/send',{
      method:'POST',body:JSON.stringify({roomId:forwardTarget.value,text:quote.slice(0,4000)})
    })
    await acceptMessage(msg)
    status.value='已转发到目标会话'
    forwardSource.value=null
  }catch(err){showError(err)}
  finally{sending.value=false}
}
async function removeLocalMessage(message){
  if(!me.value||!confirm('仅删除这台设备上的消息？其他设备和收件人仍会保留。'))return
  try{
    await deleteLocalMessage(server.value,me.value.username,message.roomId,message.id)
    messages.value=messages.value.filter(m=>m.id!==message.id)
    allLocalMessages.value=allLocalMessages.value.filter(m=>!(m.id===message.id&&m.roomId===message.roomId))
    messageActions.value=''
    status.value='仅已删除本机副本（不撤回其他设备消息）'
  }catch(err){showError(err)}
}
function onChatShortcut(event){
  if((event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='k'){
    event.preventDefault()
    document.querySelector('.tg-search input')?.focus()
    mobileChat.value=false
  }
  if(event.key==='Escape'){
    messageActions.value=''
    messageSearchOpen.value=false
    forwardSource.value=null
  }
}
function insertEmoji(emoji){
  draft.value += emoji
  emojiOpen.value=false
}
function fileSize(bytes){
  return bytes>=1024*1024 ? (bytes/1024/1024).toFixed(1)+' MB' : Math.max(1,Math.ceil(bytes/1024))+' KB'
}
async function uploadAttachment(event){
  const file=event.target.files?.[0]
  event.target.value=''
  if(file)await sendAttachmentFile(file)
}
async function sendAttachmentFile(file){

  if(!file||!me.value)return
  if(file.size>10*1024*1024){showError(Error('单个文件最多10MB'));return}
  if(!file.size){showError(Error('不能发送空文件'));return}
  if(uploadBusy.value)return
  uploadBusy.value=true
  const roomID=selectedId.value
  try{
    const form=new FormData()
    form.append('roomId',roomID)
    form.append('file',file,file.name)
    const response=await fetch(server.value+'/api/files/upload',{
      method:'POST',headers:{Authorization:'Bearer '+token.value},body:form,cache:'no-store'
    })
    const data=await response.json().catch(()=>({}))
    if(!response.ok)throw Error(data.error||'文件上传失败（HTTP '+response.status+'）')
    await acceptMessage(data)
  }catch(err){showError(err)}
  finally{uploadBusy.value=false}
}
async function showAttachment(attachment){
  if(!attachment?.id)return
  if(!/^(image\/(png|jpeg|gif|webp)|audio\/|video\/)/i.test(attachment.mime||'')){
    await downloadAttachment(attachment)
    return
  }
  try{
    const res=await fetch(server.value+'/api/files/'+encodeURIComponent(attachment.id),{
      headers:{Authorization:'Bearer '+token.value},cache:'no-store'
    })
    if(!res.ok)throw Error('查看媒体失败：HTTP '+res.status)
    const blob=await res.blob()
    closeMediaPreview()
    const url=URL.createObjectURL(blob)
    mediaPreview.value={url,mime:attachment.mime,name:attachment.name}
  }catch(err){showError(err)}
}
function closeMediaPreview(){
  if(mediaPreview.value?.url)URL.revokeObjectURL(mediaPreview.value.url)
  mediaPreview.value=null
}
async function handleFileDrop(event){
  dragOverChat.value=false
  const file=event.dataTransfer?.files?.[0]
  if(file)await sendAttachmentFile(file)
}
async function pasteFile(event){
  const file=Array.from(event.clipboardData?.files||[])[0]
  if(!file)return
  event.preventDefault()
  await sendAttachmentFile(file)
}
async function downloadAttachment(attachment){
  if(!attachment?.id)return
  try{
    const res=await fetch(server.value+'/api/files/'+encodeURIComponent(attachment.id),{
      headers:{Authorization:'Bearer '+token.value},cache:'no-store'
    })
    if(!res.ok)throw Error('下载失败：HTTP '+res.status)
    const blob=await res.blob()
    const href=URL.createObjectURL(blob)
    const a=document.createElement('a')
    a.href=href
    a.download=attachment.name||'attachment'
    document.body.appendChild(a);a.click();a.remove()
    setTimeout(()=>URL.revokeObjectURL(href),60000)
  }catch(err){showError(err)}
}

const serverName = computed(() => { try { return new URL(server.value).host } catch { return server.value } })
const sortedMessages = computed(() => messages.value.filter(m=>m.type!=='reaction').sort((a,b)=>a.createdAt.localeCompare(b.createdAt)))
const reactionSummaries = computed(()=>summarizeReactions(allLocalMessages.value,selectedId.value,me.value?.username||''))
const reactionsFor = id => reactionSummaries.value[id]||[]
const messageDay = timestamp => {
  const date=new Date(timestamp)
  if(Number.isNaN(date.getTime()))return ''
  return date.toLocaleDateString('zh-CN',{year:'numeric',month:'long',day:'numeric'})
}
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
  if (next === '/gift' || next === '/money' || next === '/api-key') mobileChat.value = true
  if(next==='/api-key' && me.value) void refreshAPIKeys()
  if(next!=='/api-key') dismissNewAPIKey()
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
  let response
  try {
    response = await fetch(server.value + path, {
      ...options,
      headers: { ...(options.body ? { 'Content-Type': 'application/json' } : {}), ...(token.value ? { Authorization: `Bearer ${token.value}` } : {}), ...options.headers },
      cache: 'no-store'
    })
  } catch (err) {
    throw Error('网络请求失败，请检查后端服务、HTTPS 和 Origin 白名单：' + (err?.message || String(err)))
  }
  const body = await response.json().catch(() => ({}))
  if (!response.ok) {
    const detail = body.error || (response.status === 403
      ? '服务器拒绝访问。请检查邀请码及 ALLOWED_ORIGINS 是否包含当前网页地址。'
      : 'HTTP ' + response.status)
    throw Error(detail)
  }
  return body
}
async function testServer() {
  busy.value = true
  status.value = ''
  try {
    const address = normalizeAddress(editedServer.value)
    // Some Safari versions throw an opaque DOMException on AbortSignal.timeout
    // or when a proxy responds with HTML instead of JSON.
    // Avoid the optional timeout API here and report the stage and HTTP code.
    let response
    try {
      const healthPath = address === location.origin ? '/api/health' : address + '/api/health'
      response = await fetch(healthPath, { cache: 'no-store' })
    } catch (err) {
      throw Error('无法连接服务器，请检查 HTTPS / DDNSTO 代理和证书：' + (err?.message || String(err)))
    }
    if (!response.ok) throw Error('服务器健康检查失败：HTTP ' + response.status)
    let data
    try {
      data = await response.json()
    } catch {
      throw Error('服务器未返回有效的 JSON；请检查 DDNSTO 是否代理了 /api/health，而非返回登录页面或网关提示。')
    }
    if (data.app !== 'ZzzMessage') throw Error('该地址不是 ZzzMessage 后端')
    disconnect()
    server.value = address
    editedServer.value = address
    localStorage.setItem(SERVER_KEY, address)
    checkedServer.value = true
    configured.value = true
    token.value = localStorage.getItem(savedTokenKey()) || ''
    await resume()
  } catch (err) {
    showError(err)
  } finally {
    busy.value = false
  }
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
    if (data.dailyReward) {
      dailyNotice.value = '今天登录获得 1 枚' + giftCurrencyName(data.dailyReward.currency)
      economyStatus.value = dailyNotice.value
      status.value = dailyNotice.value
    }
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
  restoreChatPreferences()
  await rebuildHistoryIndex()
  await refresh()
  await refreshServerMutes()
  if (!rooms.value.some(r => r.id === selectedId.value)) selectedId.value = 'lobby'
  await selectRoom(selectedId.value)
  mobileChat.value = false
  localDeviceID()
  apiConnected.value = true
  connect()
  startPendingPolling()
  void autoDailyCheckIn()
  void readPreviewPreference()
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
  const sequence = ++roomLoadSequence
  persistRoomDraft(selectedId.value,draft.value)
  selectedId.value = id
  draft.value=readRoomDraft(id)
  replyTo.value=null
  messageActions.value=''
  messageSearchOpen.value=false
  messageSearch.value=''
  infoOpen.value = false
  emojiOpen.value = false
  navigate('/message')
  mobileChat.value = true
  messages.value = []
  if (!me.value) return
  try {
    const loaded = await loadMessages(server.value,me.value.username,id)
    if (sequence !== roomLoadSequence || selectedId.value !== id) return
    // Merge arrivals that occurred while IndexedDB was loading; never
    // overwrite or lose a live message already persisted by the receiver.
    const unique = new Map(loaded.map(m=>[m.id,m]))
    for (const m of messages.value) if(m.roomId===id) unique.set(m.id,m)
    messages.value = Array.from(unique.values())
  } catch (err) { showError(err) }
  await scrollBottom()
  markRoomRead(id)
}
async function scrollBottom() { await nextTick(); if (chatScroll.value) chatScroll.value.scrollTop = chatScroll.value.scrollHeight }
function disconnect() {
  clearInterval(pollingTimer)
  pollingTimer = null
  apiConnected.value = false
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
        for(const message of data.messages)localMessageAdded(message)
        const seen=new Set(messages.value.map(m=>m.id))
        let added=false
        for (const msg of data.messages) {
          if (msg.roomId===selectedId.value && !seen.has(msg.id)) {
            messages.value.push(msg)
            seen.add(msg.id)
            added=true
          }
        }
        if (added) await scrollBottom()
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
      void rebuildHistoryIndex()
      syncBusy.value=false
    }
    else if (data.type === 'gift_received') {
      status.value = '收到 '+data.from+' 赠送的 '+data.itemName
      if (routePath.value==='/gift') void openGiftShop()
    }
    else if (data.type === 'error') status.value = data.error
    else if (data.type === 'message' || data.type === 'reaction') {
      await acceptMessage(data)
    }
  }
  ws.onclose = () => {
    if (generation !== liveSession) return
    connection.value = 'offline'
    retryTimer = setTimeout(() => { reconnectDelay = Math.min(reconnectDelay * 1.7, 15000); connect() }, reconnectDelay)
  }
  ws.onerror = () => { if (generation === liveSession) connection.value = 'offline' }
}
// Polling runs alongside WebSocket. DDNSTO and mobile networks may deliver
// Web Push correctly while blocking long-lived WebSocket upgrades.
function startPendingPolling(){
  clearInterval(pollingTimer)
  pollingTimer=setInterval(()=>{void pollPendingMessages()},4000)
  void pollPendingMessages()
}
async function acceptMessage(message){
  if(!me.value || !message?.id || !message?.roomId) return
  const account=me.value.username
  const origin=server.value
  try {
    await keepMessage(origin,account,message)
  } catch(err) {
    showError(Error('本地保存聊天记录失败：'+(err?.message || err)))
    return
  }
  if (me.value?.username!==account || server.value!==origin) return
  localMessageAdded(message)
  if(message.roomId===selectedId.value && !messages.value.some(m=>m.id===message.id)){
    messages.value.push(message)
    await scrollBottom()
    markRoomRead(message.roomId)
  }
  // ACK only after IndexedDB commit; on failure the server replays safely.
  try {
    await request('/api/messages/ack',{
      method:'POST',body:JSON.stringify({id:message.id})
    })
  } catch {
    if(socket?.readyState===WebSocket.OPEN) {
      try {socket.send(JSON.stringify({type:'ack',id:message.id}))} catch {}
    }
  }
}
async function pollPendingMessages(){
  if(pollingBusy || !token.value || !me.value || document.visibilityState==='hidden') return
  pollingBusy=true
  try {
    const reply=await request('/api/messages/pending')
    apiConnected.value=true
    if(Array.isArray(reply.rooms)) rooms.value=reply.rooms
    if(Array.isArray(reply.messages)){
      for(const msg of reply.messages){await acceptMessage(msg)}
    }
  }catch(err){
    apiConnected.value=false
    // Avoid an alarming toast every four seconds while offline.
    if(connection.value!=='online' && !status.value)
      status.value='HTTP 消息同步不可用：'+(err?.message||err)
  }finally{pollingBusy=false}
}
async function readPreviewPreference(){
  try{
    const v=await request('/api/push/preferences')
    pushPreviewEnabled.value=v.preview!==false
  }catch{}
}
async function savePreviewPreference(enabled){
  try{
    const result=await request('/api/push/preferences',{
      method:'PUT',body:JSON.stringify({preview:enabled})
    })
    pushPreviewEnabled.value=result.preview!==false
    pushStatus.value=pushPreviewEnabled.value?'通知将显示发送人及消息摘要':'通知已隐藏聊天正文'
  }catch(err){pushStatus.value=err?.message||String(err)}
}
function onPushOpen(event){
  if(event.data?.type!=='zzzmessage:open-room')return
  const id=event.data.roomId
  if(rooms.value.some(r=>r.id===id))void selectRoom(id)
  void pollPendingMessages()
}
function requestHistorySync() {
  if (!socket || socket.readyState !== WebSocket.OPEN || !me.value) {
    syncStatus.value = '需要先保持在线'
    return
  }
  syncBusy.value = true
  syncStatus.value = '正在请求在线设备同步历史…'
  socket.send(JSON.stringify({type:'sync_request'}))
  setTimeout(() => {
    if (syncBusy.value) {
      syncBusy.value=false
      syncStatus.value='历史同步等待过久，可点击重新同步。'
    }
  },30*60*1000)
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
async function autoDailyCheckIn() {
  if (!me.value) return
  try {
    const reply = await request('/api/money/daily', { method: 'POST' })
    wallet.value = reply.wallet
    if (reply.claimed) {
      dailyNotice.value = '今天登录获得 1 枚' + giftCurrencyName(reply.reward.currency)
      economyStatus.value = dailyNotice.value
      status.value = dailyNotice.value
    }
  } catch(err) {
    economyStatus.value = '自动登录奖励暂时失败：' + err.message
  }
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
async function send() {
  const text=draft.value.trim()
  if(!text || [...text].length>4000 || !me.value) return
  const roomID=selectedId.value
  if(sending.value)return
  sending.value=true
  try {
    // HTTP is compatible with all HTTPS reverse proxies; WebSocket remains
    // an optional low-latency delivery channel.
    const message=await request('/api/messages/send',{
      method:'POST',body:JSON.stringify({roomId:roomID,text,...(replyTo.value?.id?{replyTo:replyTo.value.id}:{})})
    })
    draft.value=''
    replyTo.value=null
    await acceptMessage(message)
  }catch(err){showError(err)}
  finally{sending.value=false}
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
    if (!rooms.value.some(r=>r.id===room.id)) rooms.value=[...rooms.value,room]
    await selectRoom(room.id)
    await refresh()
    // The HTTP pending endpoint also distributes the latest room list to
    // all members if a reverse proxy blocks WebSocket upgrades.
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
  pushExistingSubscription = null
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
    pushExistingSubscription = subscription
    if (subscription && matchingPushKey(subscription) && Notification.permission === 'granted') {
      await request('/api/push/subscription', { method: 'PUT', body: JSON.stringify(pushSubscriptionPayload(subscription)) })
      pushEnabled.value = true
      pushStatus.value = '本设备已启用消息通知'
    } else if (subscription && !matchingPushKey(subscription)) {
      pushStatus.value = '检测到其他后端的推送订阅，请先「重置本机推送」。'
    } else {
      pushStatus.value = '点击按钮允许系统通知。'
    }
  } catch (err) { pushStatus.value = err.message || '推送初始化失败' }
}
// PushSubscription.toJSON() also contains expirationTime; the server stores
// only the delivery endpoint and cryptographic key material.
function pushSubscriptionPayload(subscription) {
  const data = subscription.toJSON()
  return {
    endpoint: data.endpoint,
    keys: { p256dh: data.keys?.p256dh, auth: data.keys?.auth }
  }
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
    // Reuse the existing subscription for the same VAPID key. Calling
    // subscribe() with an existing mismatched key fails in Edge/Chromium.
    const pending = pushExistingSubscription && matchingPushKey(pushExistingSubscription)
      ? Promise.resolve(pushExistingSubscription)
      : pushRegistration.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: decodeVapid(pushPublicKey.value)
        })
    pending.then(async subscription => {
      pushExistingSubscription = subscription
      await request('/api/push/subscription', { method: 'PUT', body: JSON.stringify(pushSubscriptionPayload(subscription)) })
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
    pushExistingSubscription = null
    pushEnabled.value = false
    if (!pushStatus.value.includes('旧服务器')) pushStatus.value = '本设备通知已关闭'
  } catch (err) { pushStatus.value = '关闭推送失败：' + err.message }
  finally { pushBusy.value = false }
}
async function sendTestPush() {
  pushStatus.value = ''
  try {
    const reply = await request('/api/push/test', { method: 'POST' })
    if (!reply.queued) {
      pushStatus.value = '当前没有可用的推送订阅。'
      return
    }
    pushStatus.value = '已提交 ' + reply.queued + ' 条测试推送，正在等待推送服务响应…'
    // /api/push/test returns an asynchronous queue result, NOT proof
    // of delivery. Show the provider HTTP result after the workers finish.
    for (let attempt = 0; attempt < 12; attempt++) {
      await new Promise(resolve => setTimeout(resolve, 1000))
      if (!me.value) break
      const state = await request('/api/push/test-status')
      if (state.id !== reply.testId) break
      if (state.done < state.queued) continue
      if (state.failed > 0) {
        const reason = state.reason ? '，错误：' + state.reason : ''
        let hint = ''
        if (state.lastStatus === 403) {
          hint = '。Apple 返回认证失败，请检查 VAPID_SUBJECT 是否为有效的邮箱或 HTTPS 地址，并确认推送订阅与 VAPID 公钥对应。'
        }
        pushStatus.value = '推送服务拒绝了 ' + state.failed + ' 条请求，HTTP ' + state.lastStatus + reason + hint
      } else {
        pushStatus.value = '推送服务已接受 ' + state.delivered + ' 条请求。若未显示通知，请检查 iPhone 的通知权限与专注模式。'
      }
      return
    }
    pushStatus.value = '服务器尚未收到完整的推送服务结果，请稍后查看服务端日志。'
  } catch (err) { pushStatus.value = '测试推送失败：' + (err?.message || String(err)) }
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
  try {
    await clearRoom(server.value,me.value.username,selectedId.value)
    allLocalMessages.value=allLocalMessages.value.filter(m=>m.roomId!==selectedId.value)
    messages.value=[]
  } catch(err){showError(err)}
}
async function logout() {
  if (isNativeAndroid) await disableNativePush()
  else await disablePush()
  disconnect()
  closeMediaPreview()
  localStorage.removeItem(savedTokenKey())
  token.value = ''; me.value = null; rooms.value = []; users.value = []; messages.value = []
  allLocalMessages.value=[];chatPrefs.value={};replyTo.value=null;messageActions.value=''
  showSettings.value = false; status.value = ''
  apiKeys.value=[];dismissNewAPIKey();keyLabel.value='';keyRecipients.value=[];keyNotice.value='';keyError.value=''
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
  window.addEventListener('keydown',onChatShortcut)
  navigator.serviceWorker?.addEventListener('message',onPushOpen)
  document.addEventListener('visibilitychange',onVisibilityChange)
  const initialRoomFromPush=new URLSearchParams(location.search).get('room')
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
  if (me.value) {
    navigate(wantedPath === '/login' ? '/message' : wantedPath, true)
    if(initialRoomFromPush && rooms.value.some(r=>r.id===initialRoomFromPush))
      await selectRoom(initialRoomFromPush)
  } else navigate('/login',true)
})
function onVisibilityChange(){
  if(document.visibilityState==='visible'){
    void pollPendingMessages()
    markRoomRead(selectedId.value)
  }
}
onBeforeUnmount(() => {
  navigator.serviceWorker?.removeEventListener('message',onPushOpen)
  document.removeEventListener('visibilitychange',onVisibilityChange)
  window.removeEventListener('popstate', handlePopState)
  window.removeEventListener('keydown',onChatShortcut)
  window.removeEventListener('zzz:native-push', onNativePushEvent)
  window.removeEventListener('zzz:native-push-error', onNativePushError)
  disconnect()
  closeMediaPreview()
})
</script>

<template>
  <main class="shell">
    <section v-if="!configured" class="welcome card">
      <div class="brand-icon">Z</div>
      <h1>连接你的私人服务器</h1>
      <p class="muted">使用自己部署的 Go 后端。服务器之间账号与消息互相独立。</p>
      <label class="field-label">后端地址</label>
      <input v-model="editedServer" type="url" inputmode="url" autocapitalize="off" autocomplete="url" spellcheck="false" placeholder="https://chat.example.com" @keyup.enter="testServer" />
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

    <section v-else class="app card" :class="{ 'mobile-chat': mobileChat, 'info-open': infoOpen && routePath === '/message' }">
      <aside class="sidebar">
        <header class="sidebar-head tg-sidebar-head">
          <div class="tg-mobile-heading">
            <span class="tg-mobile-brand">Z</span>
            <div><strong>聊天</strong><small>ZzzMessage · 私人服务器</small></div>
            <button type="button" class="tg-mobile-heading-action" title="新建群聊" @click="openNewGroup">✎</button>
          </div>
          <div class="tg-search-row">
            <button class="tg-round-btn" type="button" aria-label="打开菜单" @click="menuOpen=!menuOpen">☰</button>
            <label class="tg-search"><span aria-hidden="true">⌕</span><input v-model="chatSearch" type="search" placeholder="搜索聊天和联系人" aria-label="搜索聊天和联系人" /></label>
          </div>
          <div v-if="menuOpen" class="tg-menu" role="menu">
            <button @click="goMenu('/message')">💬　全部聊天</button>
            <button @click="goMenu('/friend')">👥　联系人</button>
            <button @click="openNewGroup">✚　新建群聊</button>
            <button @click="goMenu('/gift')">🎁　礼物商店</button>
            <button @click="goMenu('/money')">🪙　我的钱包</button>
            <button @click="goMenu('/setting')">⚙　设置</button>
            <button @click="goMenu('/api-key')">⌁　API Key 管理</button>
            <button @click="checkMobileUpdate();menuOpen=false">⟳　检查更新</button>
            <div class="tg-menu-foot">{{ me.displayName }} · @{{ me.username }} · UI {{ mobileBuildVersion }}</div>
          </div>
        </header>
        <div class="tg-sidebar-title">
          <strong>{{ routePath === '/friend' ? '联系人' : routePath === '/setting' ? '设置' : newGroup ? '新建群聊' : '全部聊天' }}</strong>
          <button v-if="!newGroup && !showSettings" class="tg-round-btn" title="新建群聊" @click="openNewGroup">＋</button>
        </div>
        <div v-if="!showSettings && !newGroup && tab==='chats'" class="tg-mobile-filters" role="group" aria-label="过滤聊天列表">
          <button v-for="option in mobileFilterOptions" :key="option.value" type="button"
            :class="{ selected:mobileFilter===option.value }"
            @click="mobileFilter=option.value">{{ option.label }}</button>
        </div>
        <div v-if="showSettings" class="sidebar-body">
          <h3>设置</h3>
          <p class="muted">账号：{{ me.displayName }} (@{{ me.username }})</p>
          <p class="muted">后端：{{ server }}</p>
          <div class="tg-update-panel">
            <strong>手机版界面版本：{{ mobileBuildVersion }}</strong>
            <p class="hint">检查更新不会清空聊天记录，也不会取消系统通知订阅。</p>
            <button class="secondary" @click="checkMobileUpdate">检查前端更新</button>
            <p v-if="mobileUpdateStatus" class="hint" role="status">{{ mobileUpdateStatus }}</p>
          </div>
          <div class="tg-api-settings-link">
            <strong>外部 API 发消息</strong>
            <p class="hint">为 Hermes、自动化脚本生成专属 Key，无需登录即可向指定用户发送消息。</p>
            <button class="secondary" type="button" @click="goMenu('/api-key')">管理我的 API Key →</button>
          </div>
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
          <p class="hint">系统通知默认显示发送人和消息摘要，锁屏上也可能直接显示正文；关闭下方选项可隐藏预览。</p>
          <label class="member"><input type="checkbox" :checked="pushPreviewEnabled" @change="savePreviewPreference($event.target.checked)" /> 显示通知消息预览（默认开启）</label>
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
          <button v-for="u in filteredPeople" :key="u.username" class="room-item" @click="direct(u.username)"><span class="avatar">{{ u.displayName[0] }}</span><span class="room-copy"><strong>{{ u.displayName }}</strong><small>@{{ u.username }} · 发起私聊</small></span></button>
          <p v-if="!users.length" class="empty-note">目前还没有其他成员</p>
        </div>
        <div v-else class="room-list">
          <div class="list-title">聊天会话 <button class="link" @click="openNewGroup">+ 新建群聊</button></div>
          <div v-for="r in filteredRooms" :key="r.id" role="button" tabindex="0"
            class="room-item tg-room-item" :class="{ active: selectedId === r.id }"
            @click="selectRoom(r.id)" @keydown.enter.prevent="selectRoom(r.id)">
            <span class="avatar" :class="{ group: r.type !== 'direct' }">{{ roomAvatar(r) }}</span>
            <span class="room-copy tg-room-copy">
              <span class="tg-room-name"><strong>{{ roomLabel(r) }}</strong><span v-if="roomPinned(r.id)" title="本机置顶">📌</span><span v-if="roomMuted(r.id)" title="此账号系统通知已静音">🔕</span></span>
              <small class="tg-room-preview">{{ previewRoom(r.id) }}</small>
            </span>
            <span class="tg-room-right">
              <small>{{ roomTime(r.id) }}</small>
              <span v-if="roomUnread(r.id)" class="tg-unread" :class="{ muted: roomMuted(r.id) }">{{ roomUnread(r.id)>99?'99+':roomUnread(r.id) }}</span>
              <span class="tg-room-tools">
                <button type="button" @click.stop="togglePinned(r.id)" :title="roomPinned(r.id)?'取消置顶':'本机置顶'">{{ roomPinned(r.id)?'取消置顶':'置顶' }}</button>
                <button type="button" @click.stop="toggleMuted(r.id)" :title="roomMuted(r.id)?'恢复此会话通知':'静音此会话通知（所有设备）'">{{ roomMuted(r.id)?'取消静音':'静音' }}</button>
              </span>
            </span>
          </div>
          <div v-if="chatSearch.trim() && searchedMessages.length" class="tg-global-results">
            <h4>本机消息搜索结果</h4>
            <button v-for="m in searchedMessages" :key="m.id" class="tg-search-result" @click="jumpToMessage(m)">
              <strong>{{ roomLabel(rooms.find(r=>r.id===m.roomId)) }}</strong>
              <span>{{ (m.text||m.attachment?.name||'').slice(0,95) }}</span>
            </button>
          </div>
          <p v-if="chatSearch.trim() && !filteredRooms.length && !searchedMessages.length" class="empty-note">本机没有匹配的会话或消息</p>
        </div>
        <footer class="sidebar-foot tg-server-status"><span :class="['dot', connection]"></span>{{ connection === 'online' ? '实时连接正常' : apiConnected ? 'HTTP 消息同步正常（WebSocket 未连接）' : '正在连接服务器' }} <small class="tg-build-label">UI {{ mobileBuildVersion }}</small></footer>
        <nav v-if="!newGroup" class="tg-mobile-bottom-nav" aria-label="手机版导航">
          <button type="button" :class="{selected:tab==='chats'&&!showSettings}" @click="goMenu('/message');mobileChat=false">
            <span class="tg-mobile-nav-icon">▤</span><span>聊天</span>
          </button>
          <button type="button" :class="{selected:tab==='people'&&!showSettings}" @click="goMenu('/friend')">
            <span class="tg-mobile-nav-icon">♧</span><span>联系人</span>
          </button>
          <button type="button" :class="{selected:showSettings}" @click="goMenu('/setting')">
            <span class="tg-mobile-nav-icon">⚙</span><span>设置</span>
          </button>
        </nav>
      </aside>
      
      <section v-if="routePath === '/api-key'" class="api-key-pane">
        <header class="api-key-header">
          <button type="button" class="api-key-back" @click="goMenu('/setting');mobileChat=false">‹ 返回设置</button>
          <div class="api-key-headings">
            <div class="api-key-icon">⌁</div>
            <div><h2>API 发消息</h2><p>为自动化程序管理专属发送密钥</p></div>
          </div>
        </header>
        <div class="api-key-scroll">
          <section class="api-key-block">
            <h3>接口信息</h3>
            <p>外部程序不需要登录，通过 API Key 即可发送私聊消息；发送人固定为当前账号 <strong>@{{ me.username }}</strong>。</p>
            <div class="api-key-url"><code>{{ keyEndpoint }}</code><button type="button" @click="copyKeyEndpoint">复制地址</button></div>
            <p class="api-key-warning">Key 只能发送消息，不能读取历史或管理用户。请通过 HTTPS 调用，不要放进网页前端代码或公开仓库。</p>
          </section>
          <section class="api-key-block">
            <div class="api-key-section-heading"><h3>创建新 Key</h3><span>最多 5 个</span></div>
            <p v-if="!allowSecureKeys" class="api-key-error">当前不是 HTTPS 环境，请通过 HTTPS 域名打开管理页面后创建。</p>
            <form class="api-key-form" @submit.prevent="createAPIKey">
              <label class="api-key-label" for="key-label">用途名称</label>
              <input id="key-label" v-model="keyLabel" maxlength="48" placeholder="例如：Hermes Agent 日常提醒" />
              <label class="api-key-label" for="key-expiry">有效期</label>
              <select id="key-expiry" v-model.number="keyValidDays">
                <option :value="30">30 天</option>
                <option :value="90">90 天</option>
                <option :value="365">365 天</option>
              </select>
              <label class="api-key-label">允许发送给哪些用户（至少选择 1 人）</label>
              <div class="api-key-recipients">
                <label v-for="person in users" :key="person.username" class="api-key-recipient">
                  <input v-model="keyRecipients" type="checkbox" :value="person.username" />
                  <span>{{ person.displayName }} <small>@{{ person.username }}</small></span>
                </label>
                <p v-if="!users.length">没有其他注册用户，请先让收件人注册。</p>
              </div>
              <button type="submit" class="primary" :disabled="keyBusy||!allowSecureKeys||!keyRecipients.length||!keyLabel.trim()">
                {{ keyBusy ? '正在创建…' : '生成发送 Key' }}
              </button>
            </form>
            <div v-if="keyNewSecret" class="api-key-secret" role="status">
              <strong>新 Key（仅显示这一次）</strong>
              <p>请立即复制保存。页面离开后将不会显示，服务器也无法找回。</p>
              <code>{{ keyNewSecret }}</code>
              <div class="api-key-secret-actions">
                <button class="primary" type="button" @click="copyNewAPIKey">复制 Key</button>
                <button class="secondary" type="button" @click="dismissNewAPIKey">已保存，关闭</button>
              </div>
            </div>
            <p v-if="keyError" class="api-key-error" role="alert">{{ keyError }}</p>
            <p v-if="keyNotice" class="api-key-success" role="status">{{ keyNotice }}</p>
          </section>
          <section class="api-key-block">
            <div class="api-key-section-heading"><h3>已创建的 Key</h3><button type="button" @click="refreshAPIKeys">刷新</button></div>
            <p v-if="!apiKeys.length" class="api-key-empty">尚未创建 Key。</p>
            <article v-for="entry in apiKeys" :key="entry.id" class="api-key-record">
              <div class="api-key-record-main">
                <strong>{{ entry.label }}</strong><small>后四位 ····{{ entry.suffix }}</small>
                <p>收件人：{{ entry.recipients.join('、') }}</p>
                <p>有效期至：{{ new Date(entry.expiresAt).toLocaleDateString('zh-CN') }}</p>
              </div>
              <button type="button" class="danger" :disabled="keyBusy" @click="revokeAPIKey(entry)">撤销</button>
            </article>
          </section>
          <section class="api-key-block">
            <h3>如何调用</h3>
            <p>把 YOUR_API_KEY 换成刚刚复制的密钥，recipient 换成创建时允许的用户名。</p>
            <pre class="api-key-code">{{ keyExample }}</pre>
            <p class="hint">HTTP 201 表示已保存消息并开始投递，不等于收件人已阅读或系统通知已显示。发送频率限制为每账号 25 条 / 10 秒。</p>
          </section>
        </div>
      </section>
      <section v-else-if="routePath === '/money'" class="economy-pane">
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
          <h3>每日登录奖励</h3>
          <p>每天首次登录自动随机获得 1 枚虚拟货币，每个账号按 UTC 日期每天最多一次。</p>
          <p>金币 1% · 银币 9% · 铜币 90%（三选一，单次只发 1 枚）</p>
          <p v-if="dailyNotice" class="hint" role="status">{{ dailyNotice }}</p>
          <p v-if="wallet.lastDaily" class="hint">最近领取：{{ wallet.lastDaily }} · {{ giftCurrencyName(wallet.lastReward) }} ×1</p>
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
      <div v-else class="chat-pane" :class="{ 'tg-drag-active':dragOverChat }"
        @dragover.prevent="dragOverChat=true" @dragleave.self="dragOverChat=false" @drop.prevent="handleFileDrop">
        <header class="chat-head tg-chat-head">
          <button class="mobile-back tg-back" type="button" aria-label="返回聊天列表" @click="mobileChat=false;infoOpen=false;messageActions=''">‹</button>
          <button class="tg-peer" type="button" @click="infoOpen=!infoOpen" title="查看会话资料">
            <span class="avatar tg-peer-avatar" :class="{ group: activeRoom?.type !== 'direct' }">{{ activeRoom ? roomAvatar(activeRoom) : 'Z' }}</span>
            <span class="tg-peer-details"><strong>{{ activeRoom ? roomLabel(activeRoom) : '选择聊天' }}</strong><small>{{ activeRoom ? activeSubtitle : '欢迎使用 ZzzMessage' }}</small></span>
          </button>
          <div class="tg-header-actions">
            <button class="tg-round-btn" title="搜索当前会话内容" @click="messageSearchOpen=!messageSearchOpen;messageSearch='';messageActions=''">⌕</button>
            <button class="tg-round-btn" title="会话资料" @click="infoOpen=!infoOpen">⋮</button>
          </div>
        </header>
        <div v-if="messageSearchOpen" class="tg-inchat-search">
          <input v-model="messageSearch" autofocus placeholder="搜索此聊天中的消息…" aria-label="搜索当前会话内容" />
          <button @click="messageSearchOpen=false;messageSearch=''">关闭</button>
          <div v-if="messageSearch.trim()" class="tg-inchat-results">
            <button v-for="m in inRoomSearch" :key="m.id" @click="jumpToMessage(m)">
              <small>{{ m.sender }} · {{ formatTime(m.createdAt) }}</small>
              <span>{{ (m.text||m.attachment?.name||'').slice(0,150) }}</span>
            </button>
            <p v-if="!inRoomSearch.length">没有匹配的本机消息</p>
          </div>
        </div>
        <div ref="chatScroll" class="timeline">
          <div class="info-banner">消息会先由服务器保存到待投递队列，恢复连接后补发并留在本机；服务器收到收妥确认后删除待投递副本。当前不是端到端加密。</div>
          <p v-if="sortedMessages.length === 0" class="empty-chat">还没有本地聊天记录。发送第一条消息吧。</p>
          <template v-for="(m, index) in sortedMessages" :key="m.id">
          <div v-if="index===0 || messageDay(sortedMessages[index-1].createdAt)!==messageDay(m.createdAt)" class="tg-day-divider">{{ messageDay(m.createdAt) }}</div>
          <div class="message"
            :data-message-id="m.id" :class="{ mine: m.sender === me.username, 'tg-message-highlight': highlightedId===m.id }">
            <div class="bubble tg-message-bubble">
              <div class="tg-message-top">
                <small>{{ m.sender }} · {{ formatTime(m.createdAt) }}</small>
                <button type="button" class="tg-message-options" :aria-label="'消息操作 '+m.id"
                  @click.stop="messageActions=messageActions===m.id?'':m.id">⋯</button>
              </div>
              <button v-if="m.replyTo" type="button" class="tg-quoted"
                @click="jumpToMessage(replyMessage(m.replyTo))">
                ↩ {{ replyMessage(m.replyTo)?.text?.slice(0,100) || '被回复的消息尚未同步到本机' }}
              </button>
              <p v-if="!m.attachment">{{ m.text }}</p>
              <button v-else class="tg-attachment" type="button" @click="showAttachment(m.attachment)" :title="m.attachment.name">
                <span>📎</span><span><strong>{{ m.attachment.name }}</strong><small>{{ fileSize(m.attachment.size) }} · 下载文件</small></span><span>↓</span>
              </button>
              <div v-if="reactionsFor(m.id).length" class="tg-message-reactions">
                <button v-for="reaction in reactionsFor(m.id)" :key="reaction.emoji" type="button"
                  :class="{ mine: reaction.mine }" :title="'点击切换回应 '+reaction.emoji"
                  @click="reactTo(m,reaction.emoji)">{{ reaction.emoji }} {{ reaction.count }}</button>
              </div>
              <div v-if="messageActions===m.id" class="tg-message-menu">
                <div class="tg-reaction-quick">
                  <button v-for="emoji in reactionChoices" :key="emoji" type="button"
                    :disabled="reactionBusy" @click="reactTo(m,emoji)">{{ emoji }}</button>
                </div>
                <button type="button" @click="openReply(m)">↩ 回复</button>
                <button type="button" @click="copyMessage(m)">⧉ 复制文本</button>
                <button type="button" @click="beginForward(m)">➦ 转发文字</button>
                <button type="button" @click="removeLocalMessage(m)">× 仅在本机删除</button>
              </div>
            </div>
          </div>
          </template>
        </div>
        <div class="tg-composer-area">
          <p class="tg-mobile-input-tip" v-if="connection!=='online'&&apiConnected">HTTPS 消息同步运行中</p>
          <div v-if="replyTo" class="tg-reply-composer">
            <span><strong>正在回复 {{ replyTo.sender }}</strong><small>{{ replyTo.text?.slice(0,110)||replyTo.attachment?.name }}</small></span>
            <button type="button" aria-label="取消回复" @click="replyTo=null">×</button>
          </div>
          <div v-if="emojiOpen" class="tg-emoji-panel" role="group" aria-label="表情选择器">
            <button v-for="emoji in recentEmoji" :key="emoji" type="button" @click="insertEmoji(emoji)">{{ emoji }}</button>
          </div>
          <form class="composer tg-composer" @submit.prevent="send">
            <button type="button" class="tg-round-btn" :aria-label="emojiOpen?'关闭表情':'选择表情'" @click="emojiOpen=!emojiOpen">☺</button>
            <button type="button" class="tg-round-btn" title="发送文件（最多10MB）" :disabled="uploadBusy" @click="fileInput?.click()">📎</button>
            <input ref="fileInput" type="file" class="tg-file-input" @change="uploadAttachment" />
            <textarea v-model="draft" maxlength="4000" rows="1" placeholder="输入消息…（Enter 发送，Shift+Enter 换行）"
              aria-label="消息正文" @keydown.enter.exact.prevent="send" @paste="pasteFile"></textarea>
            <button type="submit" class="tg-send-btn" :disabled="!draft.trim() || uploadBusy || sending">{{ uploadBusy ? '…' : '➤' }}</button>
          </form>
        </div>
        <div v-if="status" class="toast" role="alert">{{ status }} <button @click="status = ''" aria-label="关闭提示">×</button></div>
      </div>

      <div v-if="mediaPreview" class="tg-media-screen" @click.self="closeMediaPreview">
        <div class="tg-media-content" role="dialog" aria-label="查看聊天附件">
          <div class="tg-media-header"><strong>{{ mediaPreview.name }}</strong><button @click="closeMediaPreview">×</button></div>
          <img v-if="mediaPreview.mime.startsWith('image/')" :src="mediaPreview.url" :alt="mediaPreview.name" />
          <audio v-else-if="mediaPreview.mime.startsWith('audio/')" controls :src="mediaPreview.url"></audio>
          <video v-else-if="mediaPreview.mime.startsWith('video/')" controls :src="mediaPreview.url"></video>
        </div>
      </div>
      <div v-if="forwardSource" class="tg-modal-screen" @click.self="forwardSource=null">
        <section class="tg-modal" role="dialog" aria-label="转发消息">
          <h3>转发消息</h3>
          <p>{{ forwardSource.text?.slice(0,220) }}</p>
          <label>选择目标会话</label>
          <select v-model="forwardTarget">
            <option v-for="room in rooms" :key="room.id" :value="room.id">{{ roomLabel(room) }}</option>
          </select>
          <div class="tg-modal-actions">
            <button type="button" @click="forwardSource=null">取消</button>
            <button type="button" class="primary" :disabled="sending||!forwardTarget" @click="submitForward">转发</button>
          </div>
        </section>
      </div>
      <aside v-if="infoOpen && activeRoom && routePath === '/message'" class="tg-info">
        <header class="tg-info-top">
          <button class="tg-round-btn" @click="infoOpen=false" title="关闭">×</button>
          <strong>{{ activeRoom.type==='group' ? '群组信息' : '会话信息' }}</strong>
        </header>
        <div class="tg-info-profile">
          <span class="avatar tg-info-avatar">{{ roomAvatar(activeRoom) }}</span>
          <h2>{{ roomLabel(activeRoom) }}</h2>
          <p>{{ activeSubtitle }}</p>
        </div>
        <div class="tg-info-panel">
          <h3>会话详情</h3>
          <p>🆔　{{ activeRoom.id }}</p>
          <p>🔒　消息仅对本会话成员开放</p>
        </div>
        <div v-if="activeRoom.type==='group'" class="tg-info-panel">
          <h3>群成员 · {{ activeRoom.members.length }}</h3>
          <p v-for="person in activeRoom.members" :key="person">👤　{{ users.find(u=>u.username===person)?.displayName || person }}</p>
        </div>
        <div class="tg-info-panel">
          <h3>共享附件</h3>
          <button v-for="m in sortedMessages.filter(m=>m.attachment)" :key="m.id" class="tg-shared-file" @click="downloadAttachment(m.attachment)">
            📎 {{ m.attachment.name }}
          </button>
          <p v-if="!sortedMessages.some(m=>m.attachment)">这个会话暂无附件</p>
        </div>
      </aside>
    </section>
  </main>
</template>

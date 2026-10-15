// Client-only conversation state. Messages remain in IndexedDB; pin/mute/read
// preferences and drafts are scoped to the exact server + user account.
export function scopeKey(server, username, suffix = "prefs") {
  return "zzzmessage.ui.v2|" + encodeURIComponent(server) + "|" + encodeURIComponent(username) + "|" + suffix
}
export function conversationSummaries(history, username, prefs = {}) {
  const out = {}
  for (const message of history || []) {
    if (!message || !message.roomId || !message.createdAt || !message.id) continue
    const room = message.roomId
    if (!out[room]) out[room] = { latest: null, unread: 0 }
    if (!out[room].latest || message.createdAt > out[room].latest.createdAt) out[room].latest = message
    if (message.sender !== username && message.createdAt > (prefs[room]?.lastRead || "")) out[room].unread++
  }
  return out
}
export function sortConversations(rooms, summaries, prefs = {}, query = "", label = r => r.name) {
  const needle = query.trim().toLocaleLowerCase()
  return [...rooms].filter(r => label(r).toLocaleLowerCase().includes(needle)).sort((a,b)=>{
    const pinA = !!prefs[a.id]?.pinned
    const pinB = !!prefs[b.id]?.pinned
    if (pinA !== pinB) return pinA ? -1 : 1
    const at = summaries[a.id]?.latest?.createdAt || ""
    const bt = summaries[b.id]?.latest?.createdAt || ""
    if (at !== bt) return at > bt ? -1 : 1
    if (a.id === "lobby") return 1
    if (b.id === "lobby") return -1
    return label(a).localeCompare(label(b), "zh-CN")
  })
}
export function searchLocalMessages(history, query, allowedRooms = null, limit = 60) {
  const needle = query.trim().toLocaleLowerCase()
  if (!needle) return []
  const visible = allowedRooms ? new Set(allowedRooms) : null
  return (history || [])
    .filter(m => (!visible || visible.has(m.roomId)) &&
      [m.text, m.sender, m.attachment?.name].some(v => typeof v === "string" &&
        v.toLocaleLowerCase().includes(needle)))
    .sort((a,b) => b.createdAt.localeCompare(a.createdAt))
    .slice(0,limit)
}
export function previewOf(message, username) {
  if (!message) return "暂无消息"
  const content = message.attachment ? "[文件] " + message.attachment.name : message.text || "[消息]"
  return (message.sender === username ? "我: " : "") + content.replace(/\s+/g," ").slice(0,80)
}

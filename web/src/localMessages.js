const DATABASE = 'zzzmessage-local-v1'
const STORE = 'messages'

function openDB() {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DATABASE, 1)
    request.onupgradeneeded = () => {
      const db = request.result
      if (!db.objectStoreNames.contains(STORE)) {
        const store = db.createObjectStore(STORE, { keyPath: 'key' })
        store.createIndex('space', 'space', { unique: false })
      }
    }
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
  })
}

export async function keepMessage(server, username, message) {
  const db = await openDB()
  const space = `${server}|${username}|${message.roomId}`
  try {
    return await new Promise((resolve, reject) => {
      const tx = db.transaction(STORE, 'readwrite')
      tx.objectStore(STORE).put({ ...message, space, key: `${space}|${message.id}` })
      tx.oncomplete = resolve
      tx.onerror = () => reject(tx.error)
    })
  } finally { db.close() }
}

export async function loadMessages(server, username, roomId) {
  const db = await openDB()
  try {
    const space = `${server}|${username}|${roomId}`
    return await new Promise((resolve, reject) => {
      const tx = db.transaction(STORE, 'readonly')
      const request = tx.objectStore(STORE).index('space').getAll(space)
      request.onsuccess = () => resolve(request.result.sort((a, b) => a.createdAt.localeCompare(b.createdAt)))
      request.onerror = () => reject(request.error)
    })
  } finally { db.close() }
}

export async function clearRoom(server, username, roomId) {
  const db = await openDB()
  const space = `${server}|${username}|${roomId}`
  try {
    return await new Promise((resolve, reject) => {
      const tx = db.transaction(STORE, 'readwrite')
      const index = tx.objectStore(STORE).index('space')
      const request = index.openKeyCursor(IDBKeyRange.only(space))
      request.onsuccess = () => {
        const cursor = request.result
        if (cursor) { tx.objectStore(STORE).delete(cursor.primaryKey); cursor.continue() }
      }
      tx.oncomplete = resolve
      tx.onerror = () => reject(tx.error)
    })
  } finally { db.close() }
}


// Import history chunks atomically on this device.
export async function keepMessages(server, username, batch) {
  if (!Array.isArray(batch) || batch.length > 8) throw Error('历史同步分片超出限制')
  if (!batch.length) return
  const db = await openDB()
  try {
    return await new Promise((resolve, reject) => {
      const tx = db.transaction(STORE, 'readwrite')
      const table = tx.objectStore(STORE)
      for (const message of batch) {
        if (!message?.id || !message?.roomId || !message?.createdAt || !message?.sender || typeof message.text !== 'string') {
          tx.abort()
          return
        }
        const space = server + '|' + username + '|' + message.roomId
        table.put({ ...message, space, key: space + '|' + message.id })
      }
      tx.oncomplete = () => resolve()
      tx.onerror = () => reject(tx.error || Error('历史同步写入失败'))
      tx.onabort = () => reject(tx.error || Error('历史同步已中止'))
    })
  } finally { db.close() }
}

// Export this account's complete local IndexedDB records to another
// authenticated device of the same user. Not saved permanently in Go.
export async function loadAllMessages(server, username) {
  const db = await openDB()
  try {
    const prefix = server + '|' + username + '|'
    return await new Promise((resolve, reject) => {
      const tx = db.transaction(STORE, 'readonly')
      const result = []
      const cursor = tx.objectStore(STORE).openCursor()
      cursor.onsuccess = () => {
        const c = cursor.result
        if (!c) return
        if (typeof c.value.key === 'string' && c.value.key.startsWith(prefix)) {
          const { key, space, ...message } = c.value
          result.push(message)
        }
        c.continue()
      }
      tx.oncomplete = () => resolve(result.sort((a,b) => a.createdAt.localeCompare(b.createdAt)))
      tx.onerror = () => reject(tx.error || Error('读取聊天历史失败'))
    })
  } finally { db.close() }
}

// This deletes only this device's copy. It is NOT a Telegram-style revoke for
// other participants; server-side pending copies and other devices are unchanged.
export async function deleteLocalMessage(server, username, roomId, id) {
  const db = await openDB()
  const space = server + '|' + username + '|' + roomId
  try {
    return await new Promise((resolve,reject)=>{
      const tx=db.transaction(STORE,'readwrite')
      tx.objectStore(STORE).delete(space+'|'+id)
      tx.oncomplete=()=>resolve()
      tx.onerror=()=>reject(tx.error||Error('删除本地消息失败'))
      tx.onabort=()=>reject(tx.error||Error('本地删除已中止'))
    })
  } finally {db.close()}
}

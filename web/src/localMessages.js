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

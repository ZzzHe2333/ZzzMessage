package chat

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[*Client]bool
}

func NewHub() *Hub { return &Hub{clients: make(map[string]map[*Client]bool)} }
func (h *Hub) add(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[c.username] == nil {
		h.clients[c.username] = map[*Client]bool{}
	}
	h.clients[c.username][c] = true
}
func (h *Hub) remove(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients[c.username], c)
	if len(h.clients[c.username]) == 0 {
		delete(h.clients, c.username)
	}
}
func (h *Hub) broadcast(room Room, data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for username, connections := range h.clients {
		if !member(room, username) {
			continue
		}
		for c := range connections {
			select {
			case c.send <- data:
			default:
				c.conn.Close()
			}
		}
	}
}

type Client struct {
	conn     *websocket.Conn
	username string
	deviceID string
	send     chan []byte
	done     chan struct{}
}
type incoming struct {
	Type   string `json:"type"`
	Token  string `json:"token"`
	RoomID string `json:"roomId"`
	ID string `json:"id"`
	DeviceID string `json:"deviceId"`
	TargetDevice string `json:"targetDevice"`
	RequestID string `json:"requestId"`
	Seq int `json:"seq"`
	Messages []ChatMessage `json:"messages"`
	Text   string `json:"text"`
}

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	if !s.originAllowed(r) {
		http.Error(w, "Origin forbidden", http.StatusForbidden)
		return
	}
	upgrader := websocket.Upgrader{ReadBufferSize: 4096, WriteBufferSize: 4096, CheckOrigin: func(r *http.Request) bool { return s.originAllowed(r) }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(131072)
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var auth incoming
	if err := conn.ReadJSON(&auth); err != nil || auth.Type != "auth" {
		_ = conn.WriteJSON(map[string]string{"type": "error", "error": "必须先认证"})
		return
	}
	username, ok := s.validate(auth.Token)
	if !ok {
		_ = conn.WriteJSON(map[string]string{"type": "error", "error": "登录已失效"})
		return
	}
    deviceID := auth.DeviceID
    if !validDeviceID(deviceID) {
        deviceID,err=randomID()
        if err!=nil {return}
    }
	c := &Client{conn: conn, username: username, deviceID:deviceID, send: make(chan []byte, 64), done: make(chan struct{})}
	s.hub.add(c)
	defer s.hub.remove(c)
	defer close(c.done)
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(70 * time.Second)) })
	_ = conn.SetReadDeadline(time.Now().Add(70 * time.Second))
	go c.writeLoop(s.store.PendingFor(username))
	windowStart := time.Now()
	count := 0
	for {
		var p incoming
		if err := conn.ReadJSON(&p); err != nil {
			break
		}
		switch p.Type {
		case "message":
			if time.Since(windowStart) > 10*time.Second {
				windowStart = time.Now()
				count = 0
			}
			count++
			if count > 25 {
				c.queue(map[string]any{"type": "error", "error": "发送太频繁"})
				continue
			}
			if p.Text == "" || utf8.RuneCountInString(p.Text) > 4000 {
				c.queue(map[string]any{"type": "error", "error": "消息长度需为1-4000字"})
				continue
			}
            room,msg,err:=s.store.QueueMessage(p.RoomID,username,p.Text)
            if err!=nil {
                log.Printf("offline queue failed: %v",err)
                c.queue(map[string]any{"type":"error","error":"服务器保存消息失败"})
                continue
            }
            data,err:=json.Marshal(msg)
            if err==nil {
                s.hub.broadcast(room,data)
                s.queuePush(room,msg)
                s.queueNativePush(room,username)
            }
        case "ack":
            if err:=s.store.AckMessage(username,p.ID);err!=nil {
                log.Printf("chat ACK failed: %v",err)
            }
        case "sync_request":
            s.startHistorySync(c)
        case "sync_chunk":
            s.forwardHistoryChunk(c,p)
        case "sync_chunk_ack":
            s.forwardHistoryAck(c,p)
        case "sync_done":
            s.finishHistorySync(c,p)
		default:
			c.queue(map[string]any{"type": "error", "error": "不支持的消息类型"})
		}
	}
}
func (c *Client) queue(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	select {
	case c.send <- data:
	default:
		c.conn.Close()
	}
}
func (c *Client) writeLoop(backlog []ChatMessage) {
    writeJSON := func(v any) error {
        data,err:=json.Marshal(v)
        if err!=nil{return err}
        _=c.conn.SetWriteDeadline(time.Now().Add(10*time.Second))
        return c.conn.WriteMessage(websocket.TextMessage,data)
    }
    if writeJSON(map[string]any{"type":"ready","username":c.username})!=nil{return}
    // Write backlog in stream order: don't overflow c.send's bounded buffer.
    for _,message:=range backlog{
        select{case <-c.done:return;default:}
        if writeJSON(message)!=nil{return}
    }
    ticker:=time.NewTicker(25*time.Second)
    defer ticker.Stop()
    for{
        select{
        case <-c.done:return
        case data:=<-c.send:
            _=c.conn.SetWriteDeadline(time.Now().Add(10*time.Second))
            if err:=c.conn.WriteMessage(websocket.TextMessage,data);err!=nil{return}
        case <-ticker.C:
            _=c.conn.SetWriteDeadline(time.Now().Add(10*time.Second))
            if err:=c.conn.WriteMessage(websocket.PingMessage,nil);err!=nil{return}
        }
    }
}

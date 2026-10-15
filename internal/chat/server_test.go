package chat

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gorilla/websocket"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "storage"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(Config{Store: store, InviteCode: "invite-code-long", SessionSecret: "0123456789abcdef0123456789abcdef", AllowedOrigins: []string{"http://localhost:5173"}})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func post(t *testing.T, h http.Handler, path string, p any, token string) *httptest.ResponseRecorder {
	t.Helper()
	buf, _ := json.Marshal(p)
	req := httptest.NewRequest("POST", path, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}
func reg(t *testing.T, s *Server, username string) string {
	t.Helper()
	w := post(t, s.Handler(), "/api/auth/register", map[string]string{"username": username, "password": "password123", "inviteCode": "invite-code-long"}, "")
	if w.Code != 201 {
		t.Fatalf("register %s: %d %s", username, w.Code, w.Body.String())
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.Token
}
func TestPrivateRoomsAndNoStoredMessages(t *testing.T) {
	s := testServer(t)
	alice := reg(t, s, "alice")
	_ = reg(t, s, "bob")
	_ = reg(t, s, "carol")
	w := post(t, s.Handler(), "/api/direct", map[string]string{"username": "bob"}, alice)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var room Room
	if err := json.Unmarshal(w.Body.Bytes(), &room); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.store.RoomForUser(room.ID, "carol"); ok {
		t.Fatal("uninvited user could see DM")
	}
	path := s.store.path
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("password123")) {
		t.Fatal("plaintext password stored")
	}
	if bytes.Contains(data, []byte("message text")) {
		t.Fatal("message found in state")
	}
}
func TestInviteAndOrigin(t *testing.T) {
	s := testServer(t)
	w := post(t, s.Handler(), "/api/auth/register", map[string]string{"username": "alice", "password": "password123", "inviteCode": "bad"}, "")
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	req := httptest.NewRequest("GET", "/api/health", nil)
	req.Header.Set("Origin", "https://untrusted.example")
	resp := httptest.NewRecorder()
	s.Handler().ServeHTTP(resp, req)
	if resp.Code != 403 {
		t.Fatal(resp.Code)
	}
}
func TestWebsocketOfflineNoReplay(t *testing.T) {
	s := testServer(t)
	alice := reg(t, s, "alice")
	bob := reg(t, s, "bob")
	host := httptest.NewServer(s.Handler())
	defer host.Close()
	dial := func(token string) *websocket.Conn {
		t.Helper()
		conn, _, err := websocket.DefaultDialer.Dial("ws"+host.URL[4:]+"/api/ws", nil)
		if err != nil {
			t.Fatal(err)
		}
		if err = conn.WriteJSON(map[string]string{"type": "auth", "token": token}); err != nil {
			t.Fatal(err)
		}
		var ready map[string]any
		if err = conn.ReadJSON(&ready); err != nil {
			t.Fatal(err)
		}
		if ready["type"] != "ready" {
			t.Fatal(ready)
		}
		return conn
	}
	sender := dial(alice)
	defer sender.Close()
	if err := sender.WriteJSON(map[string]string{"type": "message", "roomId": "lobby", "text": "offline never replayed"}); err != nil {
		t.Fatal(err)
	}
	var received map[string]any
	if err := sender.ReadJSON(&received); err != nil {
		t.Fatal(err)
	}
	if received["text"] != "offline never replayed" {
		t.Fatal(received)
	}
	receiver := dial(bob)
	defer receiver.Close()
	// No backlog exists; a subsequent online message is delivered in realtime.
	if err := sender.WriteJSON(map[string]string{"type": "message", "roomId": "lobby", "text": "online"}); err != nil {
		t.Fatal(err)
	}
	if err := receiver.ReadJSON(&received); err != nil {
		t.Fatal(err)
	}
	if received["text"] != "online" {
		t.Fatal(received)
	}
}

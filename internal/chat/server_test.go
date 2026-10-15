package chat

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"net/netip"
	"time"
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
func TestPrivateRoomsAndPasswordHash(t *testing.T) {
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
func TestWebsocketOfflineReplayAndAck(t *testing.T) {
    s:=testServer(t)
    alice:=reg(t,s,"alice")
    bob:=reg(t,s,"bob")
    host:=httptest.NewServer(s.Handler())
    defer host.Close()
    dial:=func(token string)*websocket.Conn{
        t.Helper()
        conn,_,err:=websocket.DefaultDialer.Dial("ws"+host.URL[4:]+"/api/ws",nil)
        if err!=nil{t.Fatal(err)}
        _=conn.SetReadDeadline(time.Now().Add(5*time.Second))
        if err=conn.WriteJSON(map[string]string{"type":"auth","token":token});err!=nil{t.Fatal(err)}
        var ready map[string]any
        if err=conn.ReadJSON(&ready);err!=nil||ready["type"]!="ready"{t.Fatal("websocket handshake",err,ready)}
        return conn
    }
    sender:=dial(alice)
    defer sender.Close()
    text:="offline delivery text"
    if err:=sender.WriteJSON(map[string]string{"type":"message","roomId":"lobby","text":text});err!=nil{t.Fatal(err)}
    var response map[string]any
    if err:=sender.ReadJSON(&response);err!=nil{t.Fatal(err)}
    id,ok:=response["id"].(string)
    if !ok||response["text"]!=text{t.Fatal(response)}
    // Bob is offline when the message arrives, but the server holds the
    // plaintext until Bob stores it and sends ACK.
    pending:=s.store.PendingFor("bob")
    if len(pending)!=1||pending[0].ID!=id{t.Fatal("offline queue missing",pending)}
    // Persisted across store reload/restart.
    reloaded,err:=NewStore(filepath.Dir(s.store.path))
    if err!=nil||len(reloaded.PendingFor("bob"))!=1{t.Fatal("queue not persisted",err)}
    receiver:=dial(bob)
    if err:=receiver.ReadJSON(&response);err!=nil{t.Fatal(err)}
    if response["id"]!=id || response["text"]!=text{t.Fatal("replay mismatch",response)}
    _=receiver.Close() // No ACK: reconnect must replay the same ID again.
    again:=dial(bob)
    defer again.Close()
    if err:=again.ReadJSON(&response);err!=nil||response["id"]!=id{t.Fatal("expected idempotent replay",response,err)}
    if err:=again.WriteJSON(map[string]string{"type":"ack","id":id});err!=nil{t.Fatal(err)}
    deadline:=time.Now().Add(3*time.Second)
    for time.Now().Before(deadline)&&len(s.store.PendingFor("bob"))!=0{time.Sleep(10*time.Millisecond)}
    if len(s.store.PendingFor("bob"))!=0{t.Fatal("ACK did not clear Bob queue")}
    if len(s.store.PendingFor("alice"))!=1{t.Fatal("Bob ACK wrongly removed Alice queue")}
    // Another message is still delivered live over WebSocket.
    if err:=sender.WriteJSON(map[string]string{"type":"message","roomId":"lobby","text":"online"});err!=nil{t.Fatal(err)}
    if err:=again.ReadJSON(&response);err!=nil||response["text"]!="online"{t.Fatal("realtime delivery failed",err,response)}
}

func TestLANOriginPrivateOnly(t *testing.T){
    s:=testServer(t)
    s.lanOriginCIDRs=[]netip.Prefix{netip.MustParsePrefix("192.168.0.0/16")}
    for _,tc:=range []struct{origin string;status int}{
        {"http://192.168.0.192:43555",200},
        {"http://192.168.1.5:58463",200},
        {"https://jinlangofficeb.ddnsto.com",403},
        {"http://8.8.8.8:58463",403},
        {"http://192.169.0.1:58463",403},
    }{
        req:=httptest.NewRequest("GET","/api/health",nil)
        req.Header.Set("Origin",tc.origin)
        rec:=httptest.NewRecorder()
        s.Handler().ServeHTTP(rec,req)
        if rec.Code!=tc.status{t.Errorf("%s: HTTP %d expected %d",tc.origin,rec.Code,tc.status)}
    }
    base:=s.store
    _,err:=NewServer(Config{Store:base,InviteCode:"invite-code-long",SessionSecret:"0123456789abcdef0123456789abcdef",
        LANOriginCIDRs:[]string{"0.0.0.0/0"}})
    if err==nil{t.Fatal("broad LAN whitelist accepted")}
}

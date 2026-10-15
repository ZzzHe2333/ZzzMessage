package chat

import (
    "bytes"
    "encoding/json"
    "net/http/httptest"
    "path/filepath"
    "strings"
    "testing"
)

func authenticatedJSON(t *testing.T,s *Server,method,path,token string, body any)*httptest.ResponseRecorder{
 t.Helper()
 var data []byte
 if body!=nil {data,_=json.Marshal(body)}
 req:=httptest.NewRequest(method,path,bytes.NewReader(data))
 req.Header.Set("Authorization","Bearer "+token)
 if body!=nil{req.Header.Set("Content-Type","application/json")}
 w:=httptest.NewRecorder()
 s.Handler().ServeHTTP(w,req)
 return w
}

func TestHTTPFallbackSendReceiveAckDurable(t *testing.T){
 s:=testServer(t)
 alice:=reg(t,s,"alice")
 bob:=reg(t,s,"bob")
 carol:=reg(t,s,"carol")

 // HTTP succeeds even when no WebSocket clients are connected.
 sent:=authenticatedJSON(t,s,"POST","/api/messages/send",alice,
   map[string]string{"roomId":"lobby","text":"iPhone 在后台收到的消息"})
 if sent.Code!=201{t.Fatalf("send HTTP %d: %s",sent.Code,sent.Body.String())}
 var msg ChatMessage
 if err:=json.Unmarshal(sent.Body.Bytes(),&msg);err!=nil{t.Fatal(err)}
 if len(msg.ID)!=32||msg.Text!="iPhone 在后台收到的消息"||msg.RoomID!="lobby"{t.Fatal(msg)}

 before:=authenticatedJSON(t,s,"GET","/api/messages/pending",bob,nil)
 var pending struct{Messages []ChatMessage `json:"messages"`}
 if err:=json.Unmarshal(before.Body.Bytes(),&pending);err!=nil||len(pending.Messages)!=1||
  pending.Messages[0].ID!=msg.ID{t.Fatalf("offline pending: %+v %v",pending,err)}

 // Nothing removes the server copy until the iPhone explicitly ACKs.
 reloaded,err:=NewStore(filepath.Dir(s.store.path))
 if err!=nil||len(reloaded.PendingFor("bob"))!=1{t.Fatal("offline copy not durable",err)}
 ack:=authenticatedJSON(t,s,"POST","/api/messages/ack",bob,map[string]string{"id":msg.ID})
 if ack.Code!=200{t.Fatal("ack failed",ack.Code,ack.Body.String())}
 ack=authenticatedJSON(t,s,"POST","/api/messages/ack",bob,map[string]string{"id":msg.ID})
 if ack.Code!=200{t.Fatal("retry ACK should be idempotent",ack.Code)}
 if got:=s.store.PendingFor("bob");len(got)!=0{t.Fatal("ack did not clear Bob",got)}
 if got:=s.store.PendingFor("alice");len(got)!=1{t.Fatal("Bob ACK cleared Alice's copy")}
 if got:=s.store.PendingFor("carol");len(got)!=1{t.Fatal("Bob ACK cleared Carol's copy")}
 noAuth:=authenticatedJSON(t,s,"GET","/api/messages/pending","",nil)
 if noAuth.Code!=401{t.Fatal("unauthenticated history allowed")}

 // Room membership remains enforced for HTTP fallback and remote clients.
 room,err:=s.store.CreateDirect("alice","bob")
 if err!=nil{t.Fatal(err)}
 denied:=authenticatedJSON(t,s,"POST","/api/messages/send",carol,
  map[string]string{"roomId":room.ID,"text":"intruder"})
 if denied.Code!=403{t.Fatalf("cross-account direct room send: %d",denied.Code)}
 private:=authenticatedJSON(t,s,"POST","/api/messages/send",alice,
  map[string]string{"roomId":room.ID,"text":"private"})
 if private.Code!=201{t.Fatalf("private send failed: %s",private.Body.String())}
 carolPending:=authenticatedJSON(t,s,"GET","/api/messages/pending",carol,nil)
 var carolMessages struct{Messages []ChatMessage `json:"messages"`}
 _=json.Unmarshal(carolPending.Body.Bytes(),&carolMessages)
 for _,m:=range carolMessages.Messages {
    if m.RoomID==room.ID{t.Fatal("private message leaked to Carol")}
 }
}

func TestHTTPFallbackRejectsInvalidPayloads(t *testing.T){
 s:=testServer(t)
 tok:=reg(t,s,"alice")
 for _,tc:=range []struct{input map[string]string;want int}{
 {map[string]string{"roomId":"unknown","text":"hi"},403},
 {map[string]string{"roomId":"lobby","text":""},400},
 {map[string]string{"roomId":"lobby","text":strings.Repeat("a",4001)},400},
 }{
  w:=authenticatedJSON(t,s,"POST","/api/messages/send",tok,tc.input)
  if w.Code!=tc.want{t.Errorf("payload %v: status %d expected %d",tc.input,w.Code,tc.want)}
 }
 // Invalid attempts also count toward rate limiting, so use a fresh
 // authenticated account to exercise exactly 25 accepted sends.
 fresh:=testServer(t)
 sender:=reg(t,fresh,"spammer")
 for n:=0;n<25;n++{
  w:=authenticatedJSON(t,fresh,"POST","/api/messages/send",sender,map[string]string{"roomId":"lobby","text":"test"})
  if w.Code!=201{t.Fatalf("send %d failed %d",n,w.Code)}
 }
 limited:=authenticatedJSON(t,fresh,"POST","/api/messages/send",sender,map[string]string{"roomId":"lobby","text":"26"})
 if limited.Code!=429{t.Fatalf("spam limit bypassed: %d",limited.Code)}
}

func TestPushPreviewDefaultOptOutAndRoomScope(t *testing.T){
 s:=testServer(t)
 alice:=reg(t,s,"alice")
 bob:=reg(t,s,"bob")
 _=alice
 if !s.store.pushPreviewAllowed("bob"){t.Fatal("preview must be enabled by default")}
 resp:=authenticatedJSON(t,s,"PUT","/api/push/preferences",bob,map[string]bool{"preview":false})
 if resp.Code!=200||s.store.pushPreviewAllowed("bob"){t.Fatal("cannot disable preview")}
 disk,err:=NewStore(filepath.Dir(s.store.path))
 if err!=nil||disk.pushPreviewAllowed("bob"){t.Fatal("preview setting not saved")}
 if !s.store.pushPreviewAllowed("alice"){t.Fatal("another account preview was affected")}
 resp=authenticatedJSON(t,s,"PUT","/api/push/preferences",bob,map[string]bool{"preview":true})
 if resp.Code!=200||!s.store.pushPreviewAllowed("bob"){t.Fatal("cannot enable preview")}
 if got:=previewText(strings.Repeat("你",150),120);len([]rune(got))!=121||!strings.HasSuffix(got,"…"){
  t.Fatal("preview was not bounded")
 }
 unauthorized:=authenticatedJSON(t,s,"GET","/api/push/preferences","",nil)
 if unauthorized.Code!=401{t.Fatal("push preferences leaked")}
}

package chat

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "path/filepath"
    "strings"
    "testing"
)

const externalTestKey = "7e4bf9b82b0def4431dfdf9517f8f1b0424d89867c40c86e064a0a3b93f6164c"

func externalPost(t *testing.T,s *Server,key string,body any) *httptest.ResponseRecorder {
    t.Helper()
    bytesBody,_:=json.Marshal(body)
    r:=httptest.NewRequest("POST","/api/external/send",bytes.NewReader(bytesBody))
    r.Header.Set("Content-Type","application/json")
    if key!=""{r.Header.Set("Authorization","Bearer "+key)}
    w:=httptest.NewRecorder()
    s.Handler().ServeHTTP(w,r)
    return w
}

func TestExternalAPIKeySendAndOfflineDelivery(t *testing.T){
    s:=testServer(t)
    _=reg(t,s,"hermes_bot")
    alice:=reg(t,s,"alice")
    _=reg(t,s,"carol")
    if err:=s.ConfigureExternalSend(ExternalSendConfig{
        Key:externalTestKey,Sender:"hermes_bot",AllowedRecipients:[]string{"alice"},
    });err!=nil{t.Fatal(err)}
    noKey:=externalPost(t,s,"",map[string]string{"recipient":"alice","text":"secret"})
    if noKey.Code!=401{t.Fatalf("missing key: %d",noKey.Code)}
    wrongKey:=externalPost(t,s,strings.Repeat("x",64),map[string]string{"recipient":"alice","text":"secret"})
    if wrongKey.Code!=401{t.Fatalf("wrong key: %d",wrongKey.Code)}
    bearerWithSession:=externalPost(t,s,alice,map[string]string{"recipient":"alice","text":"secret"})
    if bearerWithSession.Code!=401{t.Fatal("normal login session should not act as external key")}
    if len(s.store.PendingFor("alice"))!=0 {t.Fatal("unauthorized request wrote a message")}
    forbidden:=externalPost(t,s,externalTestKey,map[string]string{"recipient":"carol","text":"forbidden"})
    if forbidden.Code!=403{t.Fatalf("out of scope recipient: %d",forbidden.Code)}
    notFound:=externalPost(t,s,externalTestKey,map[string]string{"recipient":"nobody","text":"text"})
    if notFound.Code!=403{t.Fatal("out of scope user should be denied")}
    self:=externalPost(t,s,externalTestKey,map[string]string{"recipient":"hermes_bot","text":"text"})
    if self.Code!=400{t.Fatal("bot must not message itself")}
    send:=externalPost(t,s,externalTestKey,map[string]string{"recipient":"alice","text":"来自 Hermes 的提醒"})
    if send.Code!=201{t.Fatalf("external send: %d %s",send.Code,send.Body.String())}
    var response struct {OK bool `json:"ok"`;MessageID string `json:"messageId"`;RoomID string `json:"roomId"`;Recipient string `json:"recipient"`}
    if err:=json.Unmarshal(send.Body.Bytes(),&response);err!=nil{t.Fatal(err)}
    if !response.OK||len(response.MessageID)!=32||!strings.HasPrefix(response.RoomID,"dm_")||response.Recipient!="alice"{t.Fatalf("invalid send response: %+v",response)}
    room,ok:=s.store.RoomForUser(response.RoomID,"alice")
    if !ok||room.Type!="direct"||!member(room,"hermes_bot"){t.Fatal("bot direct room not created")}
    if _,ok:=s.store.RoomForUser(response.RoomID,"carol");ok{t.Fatal("unauthorized account can view bot room")}
    pending:=s.store.PendingFor("alice")
    if len(pending)!=1||pending[0].Sender!="hermes_bot"||pending[0].Text!="来自 Hermes 的提醒"{t.Fatalf("recipient queue: %+v",pending)}
    reload,err:=NewStore(filepath.Dir(s.store.path))
    if err!=nil||len(reload.PendingFor("alice"))!=1{t.Fatalf("external message did not survive restart: %v",err)}
    // After client IndexedDB commit, normal ACK clears the pending copy.
    ack:=authenticatedJSON(t,s,"POST","/api/messages/ack",alice,map[string]string{"id":response.MessageID})
    if ack.Code!=200 || len(s.store.PendingFor("alice"))!=0{t.Fatal("ack failed for external message")}
}

func TestExternalAPIDisabledAndConfigSafety(t *testing.T){
    s:=testServer(t)
    if err:=s.ConfigureExternalSend(ExternalSendConfig{AllowedRecipients:[]string{""}});err!=nil{t.Fatalf("empty environment should stay disabled: %v",err)}
    disabled:=externalPost(t,s,externalTestKey,map[string]string{"recipient":"alice","text":"text"})
    if disabled.Code!=404{t.Fatal("disabled external API should return 404")}
    _=reg(t,s,"hermes_bot")
    _=reg(t,s,"alice")
    for _,cfg:=range []ExternalSendConfig{
       {Key:"short",Sender:"hermes_bot"},
       {Key:externalTestKey,Sender:"nonexistent"},
       {Key:externalTestKey,Sender:"hermes_bot",AllowedRecipients:[]string{"bad-name!"}},
       {Key:" "+externalTestKey,Sender:"hermes_bot"},
       {Key:"",Sender:"hermes_bot"},
       {Key:"",AllowedRecipients:[]string{"alice"}},
    }{
       if err:=s.ConfigureExternalSend(cfg);err==nil{t.Fatalf("invalid config accepted: %+v",cfg)}
    }
}

func TestExternalAPIInputAndPrivileges(t *testing.T){
    s:=testServer(t)
    _=reg(t,s,"notify_bot")
    _=reg(t,s,"alice")
    if err:=s.ConfigureExternalSend(ExternalSendConfig{Key:externalTestKey,Sender:"notify_bot"});err!=nil{t.Fatal(err)}
    cases:=[]struct{body any;code int}{
      {map[string]string{"recipient":"ghost","text":"hi"},404},
      {map[string]string{"recipient":"alice","text":""},400},
      {map[string]string{"recipient":"alice","text":strings.Repeat("a",4001)},400},
      {map[string]string{"recipient":"alice","text":"hi","sender":"alice"},400},
      {map[string]string{"recipient":"alice","text":"hi","roomId":"lobby"},400},
    }
    for _,c:=range cases{
        response:=externalPost(t,s,externalTestKey,c.body)
        if response.Code!=c.code{t.Fatalf("payload %+v returned %d expected %d",c.body,response.Code,c.code)}
    }
    // No API token grants permission to call a session-authenticated route.
    r:=httptest.NewRequest(http.MethodGet,"/api/me",nil)
    r.Header.Set("Authorization","Bearer "+externalTestKey)
    w:=httptest.NewRecorder();s.Handler().ServeHTTP(w,r)
    if w.Code!=401{t.Fatal("external key should not authenticate to /api/me")}
}

package chat

import (
  "encoding/json"
  "os"
  "path/filepath"
  "strings"
  "testing"
  "time"
)

func TestManagedAPIKeyCreationSendRevokeAndRestart(t *testing.T){
  s:=testServer(t)
  alice:=reg(t,s,"alice")
  _=reg(t,s,"bob")
  carol:=reg(t,s,"carol")
  _=reg(t,s,"dave")
  settings:=map[string]any{"label":"Hermes alert","recipients":[]string{"bob"},"validDays":90}
  denied:=authenticatedJSON(t,s,"POST","/api/keys","",settings)
  if denied.Code!=401{t.Fatal("API key creation without login",denied.Code)}
  created:=authenticatedJSON(t,s,"POST","/api/keys",alice,settings)
  if created.Code!=201{t.Fatalf("create key: %d %s",created.Code,created.Body.String())}
  var result struct {Key PublicAPIKey `json:"key"`; Secret string `json:"secret"`}
  if err:=json.Unmarshal(created.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
  if !strings.HasPrefix(result.Secret,"zzm_")||len(result.Secret)!=68 {t.Fatal("invalid generated key")}
  if result.Key.Label!="Hermes alert"||len(result.Key.Recipients)!=1||result.Key.Recipients[0]!="bob" {t.Fatal(result.Key)}
  if strings.Contains(created.Body.String(),"hash") {t.Fatal("key hash leaked")}
  listed:=authenticatedJSON(t,s,"GET","/api/keys",alice,nil)
  if listed.Code!=200||strings.Contains(listed.Body.String(),result.Secret)||strings.Contains(listed.Body.String(),"hash") {t.Fatal("list exposed secret or hash")}
  // Another account cannot list or revoke this key.
  otherList:=authenticatedJSON(t,s,"GET","/api/keys",carol,nil)
  if strings.Contains(otherList.Body.String(),result.Key.ID){t.Fatal("other user saw key")}
  otherRevoke:=authenticatedJSON(t,s,"DELETE","/api/keys/"+result.Key.ID,carol,nil)
  if otherRevoke.Code!=404{t.Fatal("other user revoked key",otherRevoke.Code)}
  // The account-based managed key works with no server .env global key.
  valid:=externalPost(t,s,result.Secret,map[string]string{"recipient":"bob","text":"测试自动发送"})
  if valid.Code!=201{t.Fatal("managed key send",valid.Code,valid.Body.String())}
  var message ChatMessage
  var sendResult struct {MessageID string `json:"messageId"`;RoomID string `json:"roomId"`}
  if err:=json.Unmarshal(valid.Body.Bytes(),&sendResult);err!=nil{t.Fatal(err)}
  for _,m:=range s.store.PendingFor("bob"){if m.ID==sendResult.MessageID{message=m}}
  if message.Sender!="alice"||message.Text!="测试自动发送"{t.Fatal("sender impersonation/invalid delivery",message)}
  if denied:=externalPost(t,s,result.Secret,map[string]string{"recipient":"dave","text":"not allowed"});denied.Code!=403{t.Fatal("unscoped recipient accepted",denied.Code)}
  if denied:=externalPost(t,s,result.Secret,map[string]string{"recipient":"alice","text":"self"});denied.Code!=400{t.Fatal("self send accepted",denied.Code)}
  if denied:=externalPost(t,s,alice,map[string]string{"recipient":"bob","text":"session token"});denied.Code!=401{t.Fatal("session token impersonated key")}
  // Store contains only the SHA256 digest; restarting must not destroy the key.
  state,err:=os.ReadFile(s.store.path)
  if err!=nil{t.Fatal(err)}
  if strings.Contains(string(state),result.Secret) {t.Fatal("raw key persisted in state.json")}
  reloaded,err:=NewStore(filepath.Dir(s.store.path))
  if err!=nil{t.Fatal(err)}
  if record,ok:=reloaded.ResolveAPIKey(result.Secret);!ok||record.Owner!="alice"{t.Fatal("restarted store cannot resolve token")}
  if err:=s.store.RevokeAPIKey("alice",result.Key.ID);err!=nil{t.Fatal(err)}
  if used:=externalPost(t,s,result.Secret,map[string]string{"recipient":"bob","text":"revoked"});used.Code!=404&&used.Code!=401{t.Fatal("revoked key accepted",used.Code)}
}

func TestManagedAPIKeysValidationLimitExpiryAndLegacyCoexistence(t *testing.T){
  s:=testServer(t)
  _=reg(t,s,"service_bot")
  alice:=reg(t,s,"alice")
  _=reg(t,s,"bob")
  if err:=s.ConfigureExternalSend(ExternalSendConfig{Key:externalTestKey,Sender:"service_bot",AllowedRecipients:[]string{"bob"}});err!=nil{t.Fatal(err)}
  invalid:=[]map[string]any{
    {"label":"","recipients":[]string{"bob"},"validDays":90},
    {"label":"test","recipients":[]string{},"validDays":90},
    {"label":"test","recipients":[]string{"bob","bob"},"validDays":90},
    {"label":"test","recipients":[]string{"alice"},"validDays":90},
    {"label":"test","recipients":[]string{"bob"},"validDays":7},
    {"label":"test","recipients":[]string{"missing"},"validDays":90},
  }
  for _,body:=range invalid {
    got:=authenticatedJSON(t,s,"POST","/api/keys",alice,body)
    if got.Code!=400&&got.Code!=404{t.Fatal("invalid key accepted",got.Code,got.Body.String())}
  }
  var firstSecret string
  var firstID string
  for n:=0;n<maxAPIKeysPerAccount;n++{
    created:=authenticatedJSON(t,s,"POST","/api/keys",alice,map[string]any{
      "label":"bot "+string(rune('a'+n)),"recipients":[]string{"bob"},"validDays":30
    })
    if created.Code!=201{t.Fatal("limit create",n,created.Code,created.Body.String())}
    if n==0 {
      var result struct{Key PublicAPIKey `json:"key"`;Secret string `json:"secret"`}
      _=json.Unmarshal(created.Body.Bytes(),&result)
      firstSecret=result.Secret;firstID=result.Key.ID
    }
  }
  over:=authenticatedJSON(t,s,"POST","/api/keys",alice,map[string]any{"label":"overflow","recipients":[]string{"bob"},"validDays":30})
  if over.Code!=409{t.Fatal("limit not enforced",over.Code)}
  s.store.mu.Lock()
  record:=s.store.state.APIKeys[firstID]
  record.ExpiresAt=time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
  s.store.state.APIKeys[firstID]=record
  s.store.mu.Unlock()
  if _,ok:=s.store.ResolveAPIKey(firstSecret);ok{t.Fatal("expired key accepted")}
  if expired:=externalPost(t,s,firstSecret,map[string]string{"recipient":"bob","text":"expired"});expired.Code!=401{t.Fatal("expired key sent message",expired.Code)}
  // Existing .env integration remains functional after managed key issuance.
  legacy:=externalPost(t,s,externalTestKey,map[string]string{"recipient":"bob","text":"legacy"})
  if legacy.Code!=201{t.Fatal("env configured key broken",legacy.Code,legacy.Body.String())}
}

package chat

import (
  "bytes"
  "crypto/elliptic"
  "crypto/rand"
  "encoding/base64"
  "encoding/json"
  "path/filepath"
  "net/http/httptest"
  "net/http"
  "io"
  "strings"
  "time"
  "testing"

  webpush "github.com/SherClockHolmes/webpush-go"
)
func TestPushRegistrationAndPersistence(t *testing.T){
  s:=testServer(t)
  alice:=reg(t,s,"alice")
  bob:=reg(t,s,"bob")
  private,public,err:=webpush.GenerateVAPIDKeys()
  if err!=nil {t.Fatal(err)}
  if err:=s.ConfigurePush(public,private,"mailto:tester@example.com");err!=nil {t.Fatal(err)}
  // Cryptographic key material size must be valid; notifications are never sent in this test.
  sub:=validTestPushSubscription(t,"https://web.push.apple.com/some/device")
  if !validPushSubscription(sub) {t.Fatal("expected valid subscription")}
  r:=post(t,s.Handler(),"/api/push/test",nil,alice)
  if r.Code!=200 {t.Fatal(r.Code,r.Body.String())}
  data,_:=json.Marshal(sub)
  req:=httptest.NewRequest("PUT","/api/push/subscription",bytes.NewReader(data))
  req.Header.Set("Authorization","Bearer "+alice)
  req.Header.Set("Content-Type","application/json")
  resp:=httptest.NewRecorder()
  s.Handler().ServeHTTP(resp,req)
  if resp.Code!=200 {t.Fatalf("save subscription: %d %s",resp.Code,resp.Body.String())}
  if len(s.store.PushFor("alice"))!=1 || len(s.store.PushFor("bob"))!=0 {t.Fatal("subscription owner mismatch")}
  reloaded,err:=NewStore(filepath.Dir(s.store.path))
  if err!=nil {t.Fatal(err)}
  if len(reloaded.PushFor("alice"))!=1 {t.Fatal("subscription not persisted")}
  del:=post(t,s.Handler(),"/api/push/test",nil,bob)
  if del.Code!=200 {t.Fatal(del.Code)}
  p:=post(t,s.Handler(),"/api/push/test",nil,"")
  if p.Code!=401 {t.Fatal("missing authentication was accepted")}
}
func TestPushRejectsUntrustedTargets(t *testing.T){
  cases:=[]string{"http://fcm.googleapis.com/x","https://127.0.0.1:443/x","https://fcm.googleapis.com.evil.test/x","https://localhost/x"}
  for _,endpoint:=range cases {
    sub:=validTestPushSubscription(t,endpoint)
    if validPushSubscription(sub){t.Fatalf("SSRF endpoint accepted: %s",endpoint)}
  }
}

func validTestPushSubscription(t *testing.T, endpoint string) webpush.Subscription {
  t.Helper()
  _,x,y,err:=elliptic.GenerateKey(elliptic.P256(),rand.Reader)
  if err!=nil {t.Fatal(err)}
  key:=elliptic.Marshal(elliptic.P256(),x,y)
  return webpush.Subscription{Endpoint:endpoint,Keys:webpush.Keys{
    Auth:base64.RawURLEncoding.EncodeToString(make([]byte,16)),
    P256dh:base64.RawURLEncoding.EncodeToString(key),
  }}
}

func TestBrowserPushJSONWithExpirationTime(t *testing.T) {
 s:=testServer(t)
 alice:=reg(t,s,"alice")
 // A valid private/public pair is required to configure the push server.
 private,public,err:=webpush.GenerateVAPIDKeys()
 if err!=nil{t.Fatal(err)}
 if err:=s.ConfigurePush(public,private,"mailto:test@example.com");err!=nil{t.Fatal(err)}
 sub:=validTestPushSubscription(t,"https://web.push.apple.com/device/123")
 base,_:=json.Marshal(sub)
 var body map[string]any
 if err:=json.Unmarshal(base,&body);err!=nil{t.Fatal(err)}
 for _,expiry:=range []any{nil,float64(1790000000000)}{
   body["expirationTime"]=expiry
   bytesBody,_:=json.Marshal(body)
   request:=httptest.NewRequest("PUT","/api/push/subscription",bytes.NewReader(bytesBody))
   request.Header.Set("Authorization","Bearer "+alice)
   request.Header.Set("Content-Type","application/json")
   response:=httptest.NewRecorder()
   s.Handler().ServeHTTP(response,request)
   if response.Code!=200{t.Fatalf("browser JSON expires=%v: %d %s",expiry,response.Code,response.Body.String())}
 }
 if len(s.store.PushFor("alice"))!=1{t.Fatal("subscription was duplicated")}
 body["extraUnsafeField"]=true
 bad,_:=json.Marshal(body)
 req:=httptest.NewRequest("PUT","/api/push/subscription",bytes.NewReader(bad))
 req.Header.Set("Authorization","Bearer "+alice)
 req.Header.Set("Content-Type","application/json")
 w:=httptest.NewRecorder()
 s.Handler().ServeHTTP(w,req)
 if w.Code!=400{t.Fatalf("unknown JSON field accepted: %d",w.Code)}
}


type fakeApplePushTransport struct {code int; reply string}
func (tr fakeApplePushTransport) RoundTrip(req *http.Request)(*http.Response,error){
  return &http.Response{
    StatusCode:tr.code,
    Header:http.Header{"Apns-Id":[]string{"test-apns-id"}},
    Body:io.NopCloser(strings.NewReader(tr.reply)),
    Request:req,
  },nil
}
func TestPushTestReportsProvider403WithoutSecrets(t *testing.T){
  s:=testServer(t)
  alice:=reg(t,s,"alice")
  key,public,err:=webpush.GenerateVAPIDKeys()
  if err!=nil{t.Fatal(err)}
  if err:=s.ConfigurePush(public,key,"https://chat.example.com");err!=nil{t.Fatal(err)}
  s.push.httpClient.Transport=fakeApplePushTransport{code:403,reply:\`{"reason":"BadJwtToken"}\`}
  sub:=validTestPushSubscription(t,"https://web.push.apple.com/test/1234")
  if err:=s.store.SavePush("alice",sub);err!=nil{t.Fatal(err)}
  sent:=post(t,s.Handler(),"/api/push/test",nil,alice)
  if sent.Code!=200{t.Fatalf("test enqueue failed: %d %s",sent.Code,sent.Body.String())}
  var queued struct{Queued int \`json:"queued"\`;TestID string \`json:"testId"\`}
  if err:=json.Unmarshal(sent.Body.Bytes(),&queued);err!=nil||queued.Queued!=1||len(queued.TestID)!=32{
    t.Fatalf("queue response: %+v %v",queued,err)
  }
  done:=false
  for deadline:=time.Now().Add(3*time.Second);time.Now().Before(deadline);{
    req:=httptest.NewRequest("GET","/api/push/test-status",nil)
    req.Header.Set("Authorization","Bearer "+alice)
    resp:=httptest.NewRecorder()
    s.Handler().ServeHTTP(resp,req)
    if resp.Code!=200{t.Fatalf("poll HTTP %d",resp.Code)}
    var result PushTestResult
    if err:=json.Unmarshal(resp.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
    if result.Done==1{
      if result.LastStatus!=403||result.Failed!=1||result.Delivered!=0||result.Reason!="BadJwtToken" {
        t.Fatalf("invalid provider status %+v",result)
      }
      if strings.Contains(resp.Body.String(),sub.Endpoint)||
        strings.Contains(resp.Body.String(),key) {t.Fatal("push secrets leaked to status response")}
      done=true;break
    }
    time.Sleep(10*time.Millisecond)
  }
  if !done{t.Fatal("push worker never processed the mock Apple failure")}
  unauth:=httptest.NewRecorder()
  s.Handler().ServeHTTP(unauth,httptest.NewRequest("GET","/api/push/test-status",nil))
  if unauth.Code!=401{t.Fatal("status not protected by login")}
}
func TestPushProviderRejectReasonFiltered(t *testing.T){
  if reason:=pushProviderReason(strings.NewReader(\`{"reason":"BadJwtToken"}\`));reason!="BadJwtToken"{t.Fatal(reason)}
  for _,payload:=range []string{\`{"reason":"<img src=x>"}\`,\`{"reason":"private key material"}\`,\`{"reason":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}\`}{
    if got:=pushProviderReason(strings.NewReader(payload));got!=""{t.Fatalf("unsafe provider reason: %q",got)}
  }
}

package chat

import (
  "bytes"
  "crypto/elliptic"
  "crypto/rand"
  "encoding/base64"
  "encoding/json"
  "path/filepath"
  "net/http/httptest"
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

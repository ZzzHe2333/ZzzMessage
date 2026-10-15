package chat

import (
  "bytes"
  "encoding/json"
  "net/http/httptest"
  "strings"
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
  sub:=webpush.Subscription{Endpoint:"https://web.push.apple.com/some/device",Keys:webpush.Keys{Auth:"AAAAAAAAAAAAAAAAAAAAAA",P256dh:"BAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}}
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
  reloaded,err:=NewStore(strings.TrimSuffix(s.store.path,"/state.json"))
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
    sub:=webpush.Subscription{Endpoint:endpoint,Keys:webpush.Keys{Auth:"AAAAAAAAAAAAAAAAAAAAAA",P256dh:"BAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}}
    if validPushSubscription(sub){t.Fatalf("SSRF endpoint accepted: %s",endpoint)}
  }
}

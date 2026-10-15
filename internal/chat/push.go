package chat

import (
  "context"
  "encoding/base64"
  "encoding/json"
  "errors"
  "log"
  "net/http"
  "net/url"
  "strings"
  "sync"
  "time"

  webpush "github.com/SherClockHolmes/webpush-go"
)

// Push subscriptions are device endpoints/crypto keys, never chat contents.
// Do not allow arbitrary URLs here: web-push clients POST to the endpoint,
// so an unrestricted endpoint would be an SSRF primitive.
func validPushSubscription(v webpush.Subscription) bool {
  if len(v.Endpoint) < 20 || len(v.Endpoint) > 2048 || len(v.Keys.Auth) > 128 || len(v.Keys.P256dh) > 256 { return false }
  u, err := url.Parse(v.Endpoint)
  if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Host == "" || u.Port() != "" { return false }
  host := strings.ToLower(u.Hostname())
  allowed := host == "fcm.googleapis.com" ||
    host == "updates.push.services.mozilla.com" ||
    host == "web.push.apple.com" ||
    strings.HasSuffix(host, ".push.apple.com")
  if !allowed { return false }
  auth, e1 := base64.RawURLEncoding.DecodeString(strings.TrimRight(v.Keys.Auth, "="))
  key, e2 := base64.RawURLEncoding.DecodeString(strings.TrimRight(v.Keys.P256dh, "="))
  return e1 == nil && e2 == nil && len(auth) == 16 && len(key) == 65 && key[0] == 4
}

func (s *Store) SavePush(username string, sub webpush.Subscription) error {
  s.mu.Lock()
  defer s.mu.Unlock()
  if _, exists := s.state.Users[username]; !exists { return ErrNotFound }
  previous := s.state.Push
  copied := make(map[string][]webpush.Subscription, len(previous))
  for user, subscriptions := range previous {
    for _, v := range subscriptions {
      if v.Endpoint != sub.Endpoint {
        copied[user] = append(copied[user], v)
      }
    }
  }
  copied[username] = append(copied[username], sub)
  if len(copied[username]) > 8 { copied[username] = copied[username][len(copied[username])-8:] }
  s.state.Push = copied
  if err := s.saveLocked(); err != nil { s.state.Push = previous; return err }
  return nil
}
func (s *Store) DeletePush(username, endpoint string) error {
  s.mu.Lock()
  defer s.mu.Unlock()
  old := s.state.Push
  next := make(map[string][]webpush.Subscription, len(old))
  for u, arr := range old {
    for _, v := range arr {
      if u != username || v.Endpoint != endpoint { next[u] = append(next[u], v) }
    }
  }
  s.state.Push = next
  if err := s.saveLocked(); err != nil { s.state.Push = old; return err }
  return nil
}
func (s *Store) PushFor(username string) []webpush.Subscription {
  s.mu.RLock()
  defer s.mu.RUnlock()
  return append([]webpush.Subscription(nil), s.state.Push[username]...)
}

type pushDelivery struct {
  user string
  subscription webpush.Subscription
}
type PushService struct {
  public string
  private string
  subject string
  jobs chan pushDelivery
  httpClient *http.Client
  mu sync.Mutex
  last map[string]time.Time
}
func (s *Server) ConfigurePush(public, private, subject string) error {
  if public == "" && private == "" { return nil } // Optional feature.
  if public == "" || private == "" || ( !strings.HasPrefix(subject, "mailto:") && !strings.HasPrefix(subject, "https://") ) {
    return errors.New("Web Push requires VAPID_PUBLIC_KEY, VAPID_PRIVATE_KEY and valid VAPID_SUBJECT")
  }
  p, e1 := base64.RawURLEncoding.DecodeString(strings.TrimRight(public, "="))
  v, e2 := base64.RawURLEncoding.DecodeString(strings.TrimRight(private, "="))
  if e1 != nil || e2 != nil || len(p) != 65 || len(v) != 32 {
    return errors.New("invalid VAPID key pair format; generate with go run ./cmd/vapid")
  }
  svc := &PushService{public:public,private:private,subject:subject,jobs:make(chan pushDelivery,256),
    httpClient:&http.Client{Timeout:8*time.Second,CheckRedirect:func(_ *http.Request,_ []*http.Request) error {return http.ErrUseLastResponse}},
    last:make(map[string]time.Time)}
  s.push = svc
  for i:=0;i<4;i++ { go s.pushWorker(svc) }
  return nil
}
func (s *Server) pushConfig(w http.ResponseWriter,_ *http.Request,_ string) {
  if s.push == nil { writeJSON(w,200,map[string]any{"enabled":false}); return }
  writeJSON(w,200,map[string]any{"enabled":true,"publicKey":s.push.public})
}
func (s *Server) putPush(w http.ResponseWriter,r *http.Request,user string) {
  if s.push == nil { apiError(w,503,"服务器尚未配置 VAPID 推送"); return }
  // Browser PushSubscription.toJSON() includes expirationTime (usually null).
  // Decode only the well-defined Push API fields; retain strict JSON parsing
  // for unknown fields and validate the endpoint/keys before persistence.
  var payload struct {
    Endpoint string `json:"endpoint"`
    Keys webpush.Keys `json:"keys"`
    ExpirationTime *int64 `json:"expirationTime"`
  }
  if !readJSON(w,r,&payload) { return }
  sub:=webpush.Subscription{Endpoint:payload.Endpoint,Keys:payload.Keys}
  if !validPushSubscription(sub) { apiError(w,400,"推送订阅无效或推送服务不受支持"); return }
  if err := s.store.SavePush(user,sub); err != nil {
    log.Printf("save push subscription failed: %v",err)
    apiError(w,500,"保存推送设置失败"); return
  }
  writeJSON(w,200,map[string]bool{"ok":true})
}
func (s *Server) deletePush(w http.ResponseWriter,r *http.Request,user string) {
  var req struct {Endpoint string `json:"endpoint"` }
  if !readJSON(w,r,&req) {return}
  if len(req.Endpoint) < 20 || len(req.Endpoint)>2048 {apiError(w,400,"订阅地址无效");return}
  if err:=s.store.DeletePush(user,req.Endpoint); err!=nil {apiError(w,500,"取消订阅失败");return}
  writeJSON(w,200,map[string]bool{"ok":true})
}
func (s *Server) testPush(w http.ResponseWriter,_ *http.Request,user string) {
  if s.push==nil {apiError(w,503,"服务器未启用推送");return}
  s.push.mu.Lock()
  now:=time.Now()
  key:="test:"+user
  if now.Sub(s.push.last[key])<30*time.Second {
    s.push.mu.Unlock()
    apiError(w,429,"测试推送请间隔至少30秒")
    return
  }
  s.push.last[key]=now
  s.push.mu.Unlock()
  sent:=0
  for _,sub:=range s.store.PushFor(user) {
    if validPushSubscription(sub) && s.enqueuePush(pushDelivery{user:user,subscription:sub}) {sent++}
  }
  writeJSON(w,200,map[string]int{"queued":sent})
}
func (s *Server) enqueuePush(j pushDelivery) bool {
  if s.push==nil{return false}
  select {case s.push.jobs<-j:return true;default:return false}
}
func (s *Server) queuePush(room Room, sender string) {
  if s.push==nil{return}
  recipients:=room.Members
  if room.Type=="lobby" {
    for _,person:=range s.store.Users(sender) { recipients=append(recipients,person.Username) }
  }
  for _,username:=range recipients {
    if username==sender {continue}
    subs:=s.store.PushFor(username)
    if len(subs)==0 {continue}
    s.push.mu.Lock()
    // Coalesce frequent notifications; never enqueue message text.
    now:=time.Now()
    if now.Sub(s.push.last[username])<8*time.Second {
      s.push.mu.Unlock()
      continue
    }
    s.push.last[username]=now
    s.push.mu.Unlock()
    for _,sub:=range subs {s.enqueuePush(pushDelivery{user:username,subscription:sub})}
  }
}
func (s *Server) pushWorker(svc *PushService) {
  // The payload intentionally contains no username, group name or message text.
  payload,_:=json.Marshal(map[string]string{"title":"ZzzMessage","body":"收到一条新消息","url":"/"})
  for delivery:=range svc.jobs {
    if !validPushSubscription(delivery.subscription){continue}
    ctx,cancel:=context.WithTimeout(context.Background(),8*time.Second)
    response,err:=webpush.SendNotificationWithContext(ctx,payload,&delivery.subscription,&webpush.Options{
      Subscriber:svc.subject,VAPIDPublicKey:svc.public,VAPIDPrivateKey:svc.private,TTL:120,
      HTTPClient:svc.httpClient,
    })
    cancel()
    if err!=nil {log.Printf("web push failed for user %q: %T",delivery.user,err);continue}
    _=response.Body.Close()
    if response.StatusCode==http.StatusGone || response.StatusCode==http.StatusNotFound {
      if err:=s.store.DeletePush(delivery.user,delivery.subscription.Endpoint);err!=nil {log.Printf("prune push failed: %v",err)}
    } else if response.StatusCode<200 || response.StatusCode>=300 {
      log.Printf("web push status %d for user %q",response.StatusCode,delivery.user)
    }
  }
}

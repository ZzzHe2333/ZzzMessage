package chat

import (
  "bytes"
  "context"
  "crypto/md5"
  "encoding/hex"
  "encoding/json"
  "errors"
  "fmt"
  "io"
  "log"
  "net/http"
  "net/url"
  "regexp"
  "strconv"
  "strings"
  "sync"
  "time"
)

// The Go server saves DEVICE TOKENS, never the contents of chat messages.
// Native push is optional; vendor developer credentials and SDK access are
// external prerequisites. Only generic notifications are sent.
type NativeDevice struct {
  Provider string `json:"provider"`
  Token string `json:"token"`
}
var nativeTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_+=:./-]+$`)

func validNativeDevice(d NativeDevice) bool {
  return (d.Provider == "xiaomi" || d.Provider == "vivo") && len(d.Token) >= 12 && len(d.Token) <= 1024 && nativeTokenPattern.MatchString(d.Token)
}
func (s *Store) SaveNativeDevice(user string, device NativeDevice) error {
  s.mu.Lock()
  defer s.mu.Unlock()
  if _,ok:=s.state.Users[user]; !ok{return ErrNotFound}
  old:=s.state.NativeDevices
  next:=make(map[string][]NativeDevice,len(old))
  for username,devices:=range old{
    for _,v:=range devices {
      if v.Provider!=device.Provider || v.Token!=device.Token {
        next[username]=append(next[username],v)
      }
    }
  }
  next[user]=append(next[user],device)
  if len(next[user])>8 {next[user]=next[user][len(next[user])-8:]}
  s.state.NativeDevices=next
  if err:=s.saveLocked();err!=nil{s.state.NativeDevices=old;return err}
  return nil
}
func (s *Store) DeleteNativeDevice(user string,d NativeDevice) error {
  s.mu.Lock()
  defer s.mu.Unlock()
  old:=s.state.NativeDevices
  next:=make(map[string][]NativeDevice,len(old))
  for username,devices:=range old {
    for _,v:=range devices {
      if username!=user || v.Provider!=d.Provider || v.Token!=d.Token {
        next[username]=append(next[username],v)
      }
    }
  }
  s.state.NativeDevices=next
  if err:=s.saveLocked();err!=nil{s.state.NativeDevices=old;return err}
  return nil
}
func (s *Store) NativeFor(user string) []NativeDevice {
  s.mu.RLock();defer s.mu.RUnlock()
  return append([]NativeDevice(nil),s.state.NativeDevices[user]...)
}

type NativePushConfig struct {
  XiaomiSecret string
  XiaomiPackage string
  VivoAppID string
  VivoAppKey string
  VivoAppSecret string
}
type nativeJob struct { User string; Device NativeDevice }
type NativePushService struct {
  config NativePushConfig
  jobs chan nativeJob
  client *http.Client
  mu sync.Mutex
  last map[string]time.Time
  vivoToken string
  vivoExpiry time.Time
}
func (s *Server) ConfigureNativePush(c NativePushConfig) error {
  xm:=c.XiaomiSecret!=""||c.XiaomiPackage!=""
  vv:=c.VivoAppID!=""||c.VivoAppKey!=""||c.VivoAppSecret!=""
  // A package name by itself is harmless: it is a documented placeholder.
  if c.XiaomiSecret=="" {xm=false}
  if c.XiaomiSecret!=""&&(c.XiaomiPackage==""||!validPackage(c.XiaomiPackage)){return errors.New("XIAOMI_PACKAGE_NAME 必须与 Android applicationId 一致")}
  if vv&&(c.VivoAppID==""||c.VivoAppKey==""||c.VivoAppSecret==""){return errors.New("VIVO_APP_ID/KEY/SECRET must all be set")}
  if c.VivoAppID!="" {
    if _,err:=strconv.ParseUint(c.VivoAppID,10,64);err!=nil{return errors.New("VIVO_APP_ID must be numeric")}
  }
  if !xm&&!vv{return nil}
  svc:=&NativePushService{config:c,jobs:make(chan nativeJob,256),last:map[string]time.Time{},
    client:&http.Client{Timeout:10*time.Second,CheckRedirect:func(_ *http.Request,_ []*http.Request) error{return http.ErrUseLastResponse}}}
  s.native=svc
  for i:=0;i<4;i++{go s.nativeWorker(svc)}
  return nil
}
var packagePattern=regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(.[A-Za-z][A-Za-z0-9_]*){1,}$`)
func validPackage(s string) bool {return len(s)<200&&packagePattern.MatchString(s)}
func (s *Server) nativePushConfig(w http.ResponseWriter,_ *http.Request,_ string){
  result:=map[string]any{"xiaomi":false,"vivo":false}
  if s.native!=nil {
    result["xiaomi"]=s.native.config.XiaomiSecret!=""
    result["vivo"]=s.native.config.VivoAppID!=""
  }
  writeJSON(w,200,result)
}
func (s *Server) nativePushRegister(w http.ResponseWriter,r *http.Request,user string){
  if s.native==nil {apiError(w,503,"厂商推送未配置");return}
  var device NativeDevice
  if !readJSON(w,r,&device){return}
  if !validNativeDevice(device){apiError(w,400,"无效的设备推送令牌");return}
  if device.Provider=="xiaomi"&&s.native.config.XiaomiSecret=="" {apiError(w,503,"小米推送未启用");return}
  if device.Provider=="vivo"&&s.native.config.VivoAppID=="" {apiError(w,503,"vivo推送未启用");return}
  if err:=s.store.SaveNativeDevice(user,device);err!=nil {
    apiError(w,500,"保存推送设备失败");return
  }
  writeJSON(w,200,map[string]bool{"ok":true})
}
func (s *Server) nativePushDelete(w http.ResponseWriter,r *http.Request,user string){
  var device NativeDevice
  if !readJSON(w,r,&device){return}
  if !validNativeDevice(device){apiError(w,400,"无效的设备推送令牌");return}
  if err:=s.store.DeleteNativeDevice(user,device);err!=nil{apiError(w,500,"删除设备失败");return}
  writeJSON(w,200,map[string]bool{"ok":true})
}
func (s *Server) nativePushTest(w http.ResponseWriter,_ *http.Request,user string){
  if s.native==nil{apiError(w,503,"厂商推送未配置");return}
  s.native.mu.Lock()
  if time.Since(s.native.last["test:"+user])<30*time.Second{
    s.native.mu.Unlock();apiError(w,429,"测试间隔需30秒");return
  }
  s.native.last["test:"+user]=time.Now()
  s.native.mu.Unlock()
  queued:=0
  for _,d:=range s.store.NativeFor(user){if s.enqueueNative(nativeJob{User:user,Device:d}){queued++}}
  writeJSON(w,200,map[string]int{"queued":queued})
}
func (s *Server) enqueueNative(j nativeJob) bool {
  if s.native==nil{return false}
  select {case s.native.jobs<-j:return true;default:return false}
}
func (s *Server) queueNativePush(room Room,sender string){
  if s.native==nil{return}
  recipients:=append([]string(nil),room.Members...)
  if room.Type=="lobby" {
    for _,u:=range s.store.Users(sender){recipients=append(recipients,u.Username)}
  }
  for _,user:=range recipients{
    if user==sender||s.store.IsRoomMuted(user,room.ID){continue}
    devices:=s.store.NativeFor(user)
    if len(devices)==0{continue}
    s.native.mu.Lock()
    if time.Since(s.native.last[user])<8*time.Second{s.native.mu.Unlock();continue}
    s.native.last[user]=time.Now()
    s.native.mu.Unlock()
    for _,d:=range devices{s.enqueueNative(nativeJob{User:user,Device:d})}
  }
}
func (s *Server) nativeWorker(svc *NativePushService){
  for job:=range svc.jobs{
    ctx,cancel:=context.WithTimeout(context.Background(),12*time.Second)
    var err error
    switch job.Device.Provider {
    case "xiaomi":err=svc.sendXiaomi(ctx,job.Device.Token)
    case "vivo":err=svc.sendVivo(ctx,job.Device.Token)
    }
    cancel()
    if err!=nil{log.Printf("native push %s delivery error: %v",job.Device.Provider,err)}
  }
}
func (svc *NativePushService) sendXiaomi(ctx context.Context,regID string)error{
  if svc.config.XiaomiSecret==""{return errors.New("xiaomi not configured")}
  form:=url.Values{
    "registration_id":{regID},"restricted_package_name":{svc.config.XiaomiPackage},
    "title":{"ZzzMessage"},"description":{"收到一条新消息"},"payload":{"new_message"},
    "pass_through":{"0"},"notify_type":{"1"},"time_to_live":{"120000"},
    "extra.only_send_once":{"1"},"extra.notify_effect":{"1"},
  }
  req,err:=http.NewRequestWithContext(ctx,http.MethodPost,"https://api.xmpush.xiaomi.com/v2/message/regid",strings.NewReader(form.Encode()))
  if err!=nil{return err}
  req.Header.Set("Authorization","key="+svc.config.XiaomiSecret)
  req.Header.Set("Content-Type","application/x-www-form-urlencoded")
  response,err:=svc.client.Do(req);if err!=nil{return err}
  defer response.Body.Close()
  data,_:=io.ReadAll(io.LimitReader(response.Body,8192))
  if response.StatusCode/100!=2{return fmt.Errorf("xiaomi http %d",response.StatusCode)}
  var result struct{Code int `json:"code"`;Result string `json:"result"`}
  if err:=json.Unmarshal(data,&result);err!=nil{return fmt.Errorf("xiaomi response invalid: %w",err)}
  if result.Code!=0||result.Result!="ok"{return fmt.Errorf("xiaomi api error code %d",result.Code)}
  return nil
}
func (svc *NativePushService) sendVivo(ctx context.Context,regID string)error{
  if svc.config.VivoAppID==""{return errors.New("vivo not configured")}
  token,err:=svc.authVivo(ctx);if err!=nil{return err}
  // Intentionally no chat plaintext or sender metadata.
  payload:=map[string]any{
    "regId":regID,"notifyType":1,"title":"ZzzMessage","content":"收到一条新消息",
    "timeToLive":120,"skipType":1,
  }
  data,_:=json.Marshal(payload)
  req,err:=http.NewRequestWithContext(ctx,http.MethodPost,"https://api-push.vivo.com.cn/message/send",bytes.NewReader(data))
  if err!=nil{return err}
  req.Header.Set("Content-Type","application/json")
  req.Header.Set("authToken",token)
  resp,err:=svc.client.Do(req);if err!=nil{return err}
  defer resp.Body.Close()
  b,_:=io.ReadAll(io.LimitReader(resp.Body,4096))
  if resp.StatusCode/100!=2{return fmt.Errorf("vivo http %d",resp.StatusCode)}
  var result struct{Result int `json:"result"`}
  if err:=json.Unmarshal(b,&result);err!=nil{return err}
  if result.Result!=0{return fmt.Errorf("vivo push error %d",result.Result)}
  return nil
}
func (svc *NativePushService) authVivo(ctx context.Context)(string,error){
  svc.mu.Lock()
  defer svc.mu.Unlock()
  if svc.vivoToken!=""&&time.Now().Before(svc.vivoExpiry){return svc.vivoToken,nil}
  ts:=time.Now().UnixMilli()
  source:=strings.TrimSpace(svc.config.VivoAppID)+strings.TrimSpace(svc.config.VivoAppKey)+strconv.FormatInt(ts,10)+strings.TrimSpace(svc.config.VivoAppSecret)
  sum:=md5.Sum([]byte(source))
  appID,_:=strconv.ParseInt(svc.config.VivoAppID,10,64)
  body,_:=json.Marshal(map[string]any{
    "appId":appID,"appKey":svc.config.VivoAppKey,"timestamp":ts,"sign":hex.EncodeToString(sum[:]),
  })
  req,err:=http.NewRequestWithContext(ctx,http.MethodPost,"https://api-push.vivo.com.cn/message/auth",bytes.NewReader(body))
  if err!=nil{return "",err}
  req.Header.Set("Content-Type","application/json")
  response,err:=svc.client.Do(req);if err!=nil{return "",err}
  defer response.Body.Close()
  if response.StatusCode/100!=2{return "",fmt.Errorf("vivo auth http %d",response.StatusCode)}
  b,_:=io.ReadAll(io.LimitReader(response.Body,4096))
  var result struct{Result int `json:"result"`; AuthToken string `json:"authToken"`}
  if err:=json.Unmarshal(b,&result);err!=nil{return "",err}
  if result.Result!=0||result.AuthToken==""{return "",fmt.Errorf("vivo auth error %d",result.Result)}
  svc.vivoToken=result.AuthToken
  svc.vivoExpiry=time.Now().Add(time.Hour) // Refresh ahead of the 24h vendor TTL.
  return svc.vivoToken,nil
}

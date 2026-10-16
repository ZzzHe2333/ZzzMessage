package chat

import (
    "crypto/sha256"
    "crypto/subtle"
    "encoding/json"
    "errors"
    "log"
    "net/http"
    "strings"
    "unicode/utf8"
)

// ExternalSendConfig enables a send-only integration token. It does not issue
// user session tokens and cannot read any chat history.
type ExternalSendConfig struct {
    Key string
    Sender string
    AllowedRecipients []string
}

// ConfigureExternalSend is called before the HTTP server starts.
// The designated sender is an ordinary account created once through the UI.
// An empty key and sender leave the feature completely disabled.
func (s *Server) ConfigureExternalSend(cfg ExternalSendConfig) error {
    if cfg.Key == "" && strings.TrimSpace(cfg.Sender) == "" {
        for _,recipient:=range cfg.AllowedRecipients {
            if strings.TrimSpace(recipient)!="" {
                return errors.New("EXTERNAL_SEND_RECIPIENTS 配置后需要设置 EXTERNAL_SEND_KEY 和 EXTERNAL_SEND_SENDER")
            }
        }
        return nil
    }
    if len(cfg.Key) < 32 || len(cfg.Key) > 512 {
        return errors.New("EXTERNAL_SEND_KEY 必须是32-512位随机密钥")
    }
    if strings.TrimSpace(cfg.Key) != cfg.Key { return errors.New("EXTERNAL_SEND_KEY 不得包含首尾空格") }
    sender := strings.ToLower(strings.TrimSpace(cfg.Sender))
    if !validUsername(sender) { return errors.New("EXTERNAL_SEND_SENDER 必须是已注册的有效用户名") }
    if _, ok := s.store.GetUser(sender); !ok { return errors.New("EXTERNAL_SEND_SENDER 账号不存在，请先注册机器人账号") }
    allowed := make(map[string]bool)
    for _, raw := range cfg.AllowedRecipients {
        name := strings.ToLower(strings.TrimSpace(raw))
        if name == "" { continue }
        if !validUsername(name) { return errors.New("EXTERNAL_SEND_RECIPIENTS 包含无效用户名") }
        allowed[name] = true
    }
    s.externalKeyHash = sha256.Sum256([]byte(cfg.Key))
    s.externalSender = sender
    s.externalRecipients = allowed
    s.externalEnabled = true
    return nil
}

func (s *Server) externalSend(w http.ResponseWriter, r *http.Request) {
    if !s.externalEnabled && !s.store.HasAPIKeys(){http.NotFound(w,r);return}
    const prefix="Bearer "
    authorization:=r.Header.Get("Authorization")
    if !strings.HasPrefix(authorization,prefix)||len(authorization)>len(prefix)+512{
        w.Header().Set("WWW-Authenticate","Bearer")
        apiError(w,http.StatusUnauthorized,"无效的 API Key")
        return
    }
    supplied:=strings.TrimPrefix(authorization,prefix)
    sender:=""
    allowed:=map[string]bool{}
    managed,ok:=s.store.ResolveAPIKey(supplied)
    if ok {
        sender=managed.Owner
        for _,name:=range managed.Recipients {allowed[name]=true}
    } else if s.externalEnabled {
        digest:=sha256.Sum256([]byte(supplied))
        if subtle.ConstantTimeCompare(digest[:],s.externalKeyHash[:])==1 {
            sender=s.externalSender
            allowed=s.externalRecipients
        }
    }
    if sender=="" {
        w.Header().Set("WWW-Authenticate","Bearer")
        apiError(w,http.StatusUnauthorized,"API Key 无效、已撤销或已过期")
        return
    }
    var req struct {
        Recipient string `json:"recipient"`
        Text string `json:"text"`
    }
    if !readJSON(w,r,&req) {return}
    recipient := strings.ToLower(strings.TrimSpace(req.Recipient))
    if !validUsername(recipient) || recipient==sender {
        apiError(w,400,"收件人无效或不能给机器人自己发送");return
    }
    if !allowed[recipient] && !( !ok && s.externalEnabled && sender==s.externalSender && len(s.externalRecipients)==0 ) {
        apiError(w,403,"该 API Key 无权向此用户发送");return
    }
    if _,ok:=s.store.GetUser(recipient);!ok {
        apiError(w,404,"收件人不存在");return
    }
    if req.Text=="" || !utf8.ValidString(req.Text) || utf8.RuneCountInString(req.Text)>4000 {
        apiError(w,400,"消息长度需为1-4000字");return
    }
    // API clients cannot bypass the existing server-side send frequency cap.
    if s.limitSend("external:"+sender) {apiError(w,429,"发送太频繁");return}
    room,err:=s.store.CreateDirect(sender,recipient)
    if err!=nil {
        log.Printf("external send room creation failed: %v",err)
        apiError(w,500,"创建私聊失败");return
    }
    s.notifyRoomChanged(room)
    stored,msg,err:=s.store.QueueMessage(room.ID,sender,req.Text)
    if err!=nil {
        log.Printf("external message persistence failed: %v",err)
        apiError(w,500,"保存消息失败");return
    }
    bytes,err:=json.Marshal(msg)
    if err==nil{s.hub.broadcast(stored,bytes)}
    s.queuePush(stored,msg)
    s.queueNativePush(stored,sender)
    writeJSON(w,201,map[string]any{"ok":true,"messageId":msg.ID,"roomId":msg.RoomID,"recipient":recipient,"createdAt":msg.CreatedAt})
}

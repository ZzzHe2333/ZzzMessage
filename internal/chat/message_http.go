package chat

import (
    "encoding/json"
    "log"
    "net/http"
    "time"
    "unicode/utf8"
)

// Browser/PWA HTTP fallback for proxies that don't support WebSocket upgrades.
// Messages are never ACKed until an authenticated client has persisted them
// in IndexedDB, so a backgrounded iPhone can safely resume.
func (s *Server) httpPending(w http.ResponseWriter, _ *http.Request, user string) {
    all:=s.store.PendingFor(user)
    if len(all)>100 {all=all[:100]}
    result:=append([]ChatMessage{},all...)
    // Returning current room membership with pending messages lets a disconnected
    // client discover newly-created groups even when WebSocket Upgrade fails.
    writeJSON(w,http.StatusOK,map[string]any{"messages":result,"rooms":s.store.RoomsFor(user)})
}

func (s *Server) limitSend(user string)bool{
    now:=time.Now()
    s.sendMu.Lock()
    defer s.sendMu.Unlock()
    if len(s.sendTimes)>10000 {s.sendTimes=make(map[string][]time.Time)}
    recent:=make([]time.Time,0,25)
    for _,t:=range s.sendTimes[user] {
        if now.Sub(t)<10*time.Second {recent=append(recent,t)}
    }
    if len(recent)>=25 {
        s.sendTimes[user]=recent
        return true
    }
    s.sendTimes[user]=append(recent,now)
    return false
}
func (s *Server) httpSend(w http.ResponseWriter,r *http.Request,user string){
    var data struct{
        RoomID string `json:"roomId"`
        Text string `json:"text"`
        ReplyTo string `json:"replyTo"`
    }
    if !readJSON(w,r,&data){return}
    if data.Text==""||!utf8.ValidString(data.Text)||utf8.RuneCountInString(data.Text)>4000{
        apiError(w,400,"消息长度需为1-4000字")
        return
    }
    if data.ReplyTo!="" && !validDeviceID(data.ReplyTo) {
        apiError(w,400,"回复消息编号无效")
        return
    }
    if s.limitSend(user) {
        apiError(w,429,"发送太频繁")
        return
    }
    room,message,err:=s.store.QueueReplyMessage(data.RoomID,user,data.Text,data.ReplyTo)
    if err!=nil {
        if err==ErrInvalid||err==ErrNotFound {
            apiError(w,403,"无法向该会话发送消息")
            return
        }
        log.Printf("HTTP message persistence failed: %v",err)
        apiError(w,500,"服务器保存消息失败")
        return
    }
    payload,err:=json.Marshal(message)
    if err==nil{s.hub.broadcast(room,payload)}
    s.queuePush(room,message)
    s.queueNativePush(room,user)
    writeJSON(w,http.StatusCreated,message)
}
func (s *Server) httpAck(w http.ResponseWriter,r *http.Request,user string){
    var data struct{ID string `json:"id"`}
    if !readJSON(w,r,&data){return}
    if !validDeviceID(data.ID){apiError(w,400,"无效的消息 ID");return}
    if err:=s.store.AckMessage(user,data.ID);err!=nil{
        apiError(w,500,"确认消息失败")
        return
    }
    writeJSON(w,200,map[string]bool{"ok":true})
}


// Reactions are durable append-only toggles. A sender tapping an existing
// reaction again sends a second event; clients use parity per user/emoji/target.
// The target's actual content stays client-local, not in a cloud chat history.
func (s *Server) httpReact(w http.ResponseWriter,r *http.Request,user string){
    var input struct {
        RoomID string `json:"roomId"`
        MessageID string `json:"messageId"`
        Emoji string `json:"emoji"`
    }
    if !readJSON(w,r,&input){return}
    allowed:=map[string]bool{"👍":true,"❤️":true,"😂":true,"😮":true,"😢":true,"🎉":true}
    if !validDeviceID(input.MessageID)||!allowed[input.Emoji]{
        apiError(w,400,"无效的回应消息或表情")
        return
    }
    if s.limitSend(user){apiError(w,429,"操作过于频繁");return}
    room,message,err:=s.store.QueueReactionMessage(input.RoomID,user,input.MessageID,input.Emoji)
    if err!=nil{
        if err==ErrInvalid||err==ErrNotFound{apiError(w,403,"无权对该会话回应");return}
        log.Printf("reaction persist failed: %v",err)
        apiError(w,500,"保存回应失败")
        return
    }
    if data,err:=json.Marshal(message);err==nil{s.hub.broadcast(room,data)}
    // Emoji reactions intentionally do not generate additional Web Push.
    writeJSON(w,201,message)
}

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
    }
    if !readJSON(w,r,&data){return}
    if data.Text==""||!utf8.ValidString(data.Text)||utf8.RuneCountInString(data.Text)>4000{
        apiError(w,400,"消息长度需为1-4000字")
        return
    }
    if s.limitSend(user) {
        apiError(w,429,"发送太频繁")
        return
    }
    room,message,err:=s.store.QueueMessage(data.RoomID,user,data.Text)
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

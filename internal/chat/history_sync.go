package chat

import (
  "encoding/json"
  "regexp"
  "time"
)

// Long-term chat history stays in each device's IndexedDB. The server only
// forwards history from an online device owned by the SAME authenticated user.
// No history chunks are written to disk or logs.
var deviceIDRE=regexp.MustCompile("^[a-f0-9]{32}$")
func validDeviceID(id string) bool{return deviceIDRE.MatchString(id)}
type historyRequest struct {
  User string
  TargetDevice string
  Started time.Time
}

func (h *Hub) sendToUser(username string, payload any){
    data,err:=json.Marshal(payload);if err!=nil{return}
    h.mu.RLock();defer h.mu.RUnlock()
    for c:=range h.clients[username]{
        select{case c.send<-data:default:c.conn.Close()}
    }
}
func (h *Hub) sendToDevice(username,device string,payload any)bool{
    data,err:=json.Marshal(payload);if err!=nil{return false}
    h.mu.RLock();defer h.mu.RUnlock()
    delivered:=false
    for c:=range h.clients[username]{
        if c.deviceID!=device{continue}
        select{case c.send<-data:delivered=true;default:c.conn.Close()}
    }
    return delivered
}
func (h *Hub) askPeerDevices(user,otherThan string,payload any)int{
    data,err:=json.Marshal(payload);if err!=nil{return 0}
    h.mu.RLock();defer h.mu.RUnlock()
    seen:=map[string]bool{}
    for c:=range h.clients[user]{
        if c.deviceID==otherThan||seen[c.deviceID]{continue}
        select{case c.send<-data:seen[c.deviceID]=true;default:c.conn.Close()}
    }
    return len(seen)
}

func (s *Server) startHistorySync(c *Client){
    id,err:=randomID()
    if err!=nil{return}
    s.syncMu.Lock()
    // Prune expired in-memory forwarding permissions; bounded by 30 minutes.
    now:=time.Now()
    for id,v:=range s.historyRequests {
        if now.Sub(v.Started)>30*time.Minute {delete(s.historyRequests,id)}
    }
    // One outstanding request for a device/account at a time.
    for id,v:=range s.historyRequests{
        if v.User==c.username&&v.TargetDevice==c.deviceID {
            delete(s.historyRequests,id)
        }
    }
    if len(s.historyRequests)>1000{s.historyRequests=map[string]historyRequest{}}
    s.historyRequests[id]=historyRequest{User:c.username,TargetDevice:c.deviceID,Started:now}
    s.syncMu.Unlock()
    peers:=s.hub.askPeerDevices(c.username,c.deviceID,
        map[string]any{"type":"sync_request","requestId":id,"targetDevice":c.deviceID})
    c.queue(map[string]any{"type":"sync_status","requestId":id,"peers":peers})
}
func (s *Server) permittedHistoryRequest(c *Client,requestID,target string,requester bool)bool{
    if !validDeviceID(requestID) || !validDeviceID(target){return false}
    s.syncMu.Lock();defer s.syncMu.Unlock()
    request,ok:=s.historyRequests[requestID]
    if !ok||request.User!=c.username||request.TargetDevice!=target||time.Since(request.Started)>30*time.Minute{return false}
    if requester{return c.deviceID==request.TargetDevice}
    return c.deviceID!=request.TargetDevice
}
func (s *Server) forwardHistoryChunk(c *Client,p incoming){
    if !s.permittedHistoryRequest(c,p.RequestID,p.TargetDevice,false){return}
    if p.Seq<0 || len(p.Messages)<1 || len(p.Messages)>8{return}
    for _,m:=range p.Messages{
        if m.Type!="message"||!validDeviceID(m.ID)||len(m.Text)==0||
          len([]rune(m.Text))>4000||len(m.CreatedAt)>64{return}
        room,ok:=s.store.RoomForUser(m.RoomID,c.username)
        if !ok || !member(room,m.Sender){return}
        if _,err:=time.Parse(time.RFC3339Nano,m.CreatedAt);err!=nil{return}
    }
    s.hub.sendToDevice(c.username,p.TargetDevice,map[string]any{
        "type":"sync_chunk","requestId":p.RequestID,"sourceDevice":c.deviceID,
        "seq":p.Seq,"messages":p.Messages,
    })
}
func (s *Server) forwardHistoryAck(c *Client,p incoming){
    if !validDeviceID(p.DeviceID) || p.Seq<0 ||
      !s.permittedHistoryRequest(c,p.RequestID,c.deviceID,true){return}
    // Only the authorized requester can acknowledge a forwarded chunk.
    s.hub.sendToDevice(c.username,p.DeviceID,map[string]any{
        "type":"sync_chunk_ack","requestId":p.RequestID,"seq":p.Seq,
    })
}
func (s *Server) finishHistorySync(c *Client,p incoming){
    if !s.permittedHistoryRequest(c,p.RequestID,p.TargetDevice,false){return}
    s.hub.sendToDevice(c.username,p.TargetDevice,map[string]any{
        "type":"sync_done","requestId":p.RequestID,"sourceDevice":c.deviceID,
    })
}

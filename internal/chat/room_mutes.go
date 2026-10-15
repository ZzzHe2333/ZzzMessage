package chat

import "net/http"

// Room mute is per account (not merely a CSS flag) and suppresses new-message
// Web Push and configured native push. Delivery to the room remains intact.
func (s *Store) IsRoomMuted(username,roomID string)bool {
 s.mu.RLock()
 defer s.mu.RUnlock()
 return s.state.MutedRooms[username][roomID]
}
func (s *Store) RoomMutes(username string)map[string]bool{
 s.mu.RLock()
 defer s.mu.RUnlock()
 result:=map[string]bool{}
 for id,muted:=range s.state.MutedRooms[username]{
   if muted {
     if room,exists:=s.state.Rooms[id];exists&&member(room,username){result[id]=true}
   }
 }
 return result
}
func (s *Store) SetRoomMuted(username,roomID string,muted bool)error{
 s.mu.Lock()
 defer s.mu.Unlock()
 room,ok:=s.state.Rooms[roomID]
 if !ok||!member(room,username){return ErrInvalid}
 old:=s.state.MutedRooms
 next:=make(map[string]map[string]bool,len(old))
 for user,byRoom:=range old{
   saved:=make(map[string]bool,len(byRoom))
   for id,v:=range byRoom{saved[id]=v}
   next[user]=saved
 }
 if next[username]==nil{next[username]=map[string]bool{}}
 if muted{next[username][roomID]=true}else{delete(next[username],roomID)}
 s.state.MutedRooms=next
 if err:=s.saveLocked();err!=nil{s.state.MutedRooms=old;return err}
 return nil
}
func (s *Server) getRoomMutes(w http.ResponseWriter,_ *http.Request,user string){
 writeJSON(w,200,map[string]any{"mutedRooms":s.store.RoomMutes(user)})
}
func (s *Server) updateRoomMute(w http.ResponseWriter,r *http.Request,user string){
 var input struct {
   RoomID string `json:"roomId"`
   Muted *bool `json:"muted"`
 }
 if !readJSON(w,r,&input){return}
 if input.Muted==nil||input.RoomID==""{apiError(w,400,"缺少房间或静音状态");return}
 if err:=s.store.SetRoomMuted(user,input.RoomID,*input.Muted);err!=nil{
   if err==ErrInvalid{apiError(w,403,"无权配置该会话");return}
   apiError(w,500,"保存会话通知设置失败");return
 }
 writeJSON(w,200,map[string]any{"roomId":input.RoomID,"muted":*input.Muted})
}

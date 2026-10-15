package chat

import (
  "time"
)

const maxPendingPerAccount=500
const pendingTTL=14*24*time.Hour

// Every unacknowledged message is plaintext in state.json until acknowledged,
// expired, or displaced by the bounded offline queue.
type ChatMessage struct {
  Type string `json:"type"`
  ID string `json:"id"`
  RoomID string `json:"roomId"`
  Sender string `json:"sender"`
  Text string `json:"text"`
  CreatedAt string `json:"createdAt"`
}
func (s *Store) QueueMessage(roomID,sender,text string)(Room,ChatMessage,error){
 s.mu.Lock();defer s.mu.Unlock()
 room,ok:=s.state.Rooms[roomID]
 if !ok||!member(room,sender){return Room{},ChatMessage{},ErrInvalid}
 if _,ok:=s.state.Users[sender];!ok{return Room{},ChatMessage{},ErrNotFound}
 id,err:=randomID();if err!=nil{return Room{},ChatMessage{},err}
 now:=time.Now().UTC()
 msg:=ChatMessage{Type:"message",ID:id,RoomID:roomID,Sender:sender,Text:text,CreatedAt:now.Format(time.RFC3339Nano)}
 recipients:=room.Members
 if room.Type=="lobby"{
  recipients=make([]string,0,len(s.state.Users))
  for username:=range s.state.Users{recipients=append(recipients,username)}
 }
 old:=s.state.Pending
 next:=make(map[string][]ChatMessage,len(old))
 cutoff:=now.Add(-pendingTTL)
 for user,queue:=range old{
  for _,m:=range queue{
   timestamp,e:=time.Parse(time.RFC3339Nano,m.CreatedAt)
   if e==nil&&timestamp.After(cutoff){next[user]=append(next[user],m)}
  }
 }
 for _,user:=range recipients{
  next[user]=append(next[user],msg)
  if len(next[user])>maxPendingPerAccount{
   next[user]=append([]ChatMessage(nil),next[user][len(next[user])-maxPendingPerAccount:]...)
  }
 }
 s.state.Pending=next
 if err:=s.saveLocked();err!=nil{s.state.Pending=old;return Room{},ChatMessage{},err}
 return room,msg,nil
}
func (s *Store) PendingFor(user string)[]ChatMessage{
 s.mu.RLock();defer s.mu.RUnlock()
 after:=time.Now().Add(-pendingTTL)
 result:=make([]ChatMessage,0,len(s.state.Pending[user]))
 for _,m:=range s.state.Pending[user]{
  timestamp,e:=time.Parse(time.RFC3339Nano,m.CreatedAt)
  if e==nil&&timestamp.After(after){result=append(result,m)}
 }
 return result
}
func (s *Store) AckMessage(user,id string)error{
 if len(id)!=32{return ErrInvalid}
 s.mu.Lock();defer s.mu.Unlock()
 previous:=s.state.Pending[user]
 if len(previous)==0{return nil}
 next:=make([]ChatMessage,0,len(previous))
 removed:=false
 for _,m:=range previous{if m.ID==id{removed=true;continue};next=append(next,m)}
 if !removed{return nil}
 s.state.Pending[user]=next
 if err:=s.saveLocked();err!=nil{s.state.Pending[user]=previous;return err}
 return nil
}

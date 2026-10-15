package chat

import (
 "encoding/json"
 "net/http/httptest"
 "path/filepath"
 "testing"
)
func TestRoomMuteAccountScopeAndDiskPersistence(t *testing.T){
 s:=testServer(t)
 alice:=reg(t,s,"alice")
 _=reg(t,s,"bob")
 carol:=reg(t,s,"carol")
 room,err:=s.store.CreateDirect("alice","bob")
 if err!=nil{t.Fatal(err)}
 set:=authenticatedJSON(t,s,"PUT","/api/rooms/mutes",alice,
  map[string]any{"roomId":room.ID,"muted":true})
 if set.Code!=200{t.Fatalf("set mute failed %d %s",set.Code,set.Body.String())}
 if !s.store.IsRoomMuted("alice",room.ID){t.Fatal("missing own mute")}
 if s.store.IsRoomMuted("bob",room.ID){t.Fatal("mute leaked across accounts")}
 if got:=authenticatedJSON(t,s,"PUT","/api/rooms/mutes",carol,
    map[string]any{"roomId":room.ID,"muted":true});got.Code!=403{t.Fatal("unauthorized mute accepted")}
 reload,err:=NewStore(filepath.Dir(s.store.path))
 if err!=nil{t.Fatal(err)}
 if !reload.IsRoomMuted("alice",room.ID){t.Fatal("mute did not persist across restart")}
 get:=authenticatedJSON(t,s,"GET","/api/rooms/mutes",alice,nil)
 if get.Code!=200{t.Fatal(get.Code)}
 var got struct {MutedRooms map[string]bool `json:"mutedRooms"`}
 if err:=json.Unmarshal(get.Body.Bytes(),&got);err!=nil{t.Fatal(err)}
 if !got.MutedRooms[room.ID]{t.Fatal("GET should contain muted room")}
 off:=authenticatedJSON(t,s,"PUT","/api/rooms/mutes",alice,
  map[string]any{"roomId":room.ID,"muted":false})
 if off.Code!=200 || s.store.IsRoomMuted("alice",room.ID){t.Fatal("unmute failed")}
 noAuth:=httptest.NewRecorder()
 s.Handler().ServeHTTP(noAuth,httptest.NewRequest("GET","/api/rooms/mutes",nil))
 if noAuth.Code!=401{t.Fatal("room mute list not protected")}
 invalid:=authenticatedJSON(t,s,"PUT","/api/rooms/mutes",alice,
  map[string]any{"roomId":"unknown","muted":true})
 if invalid.Code!=403{t.Fatal("invalid room accepted")}
 // Suppressing notifications never suppresses actual messages.
 _=s.store.SetRoomMuted("bob",room.ID,true)
 _,_,err=s.store.QueueMessage(room.ID,"alice","hello muted recipient")
 if err!=nil{t.Fatal(err)}
 if len(s.store.PendingFor("bob"))!=1{t.Fatal("mute suppressed message delivery")}
}

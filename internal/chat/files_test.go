package chat

import (
  "bytes"
  "encoding/json"
  "io"
  "mime/multipart"
  "net/http"
  "net/http/httptest"
  "os"
  "path/filepath"
  "strings"
  "testing"
)

func uploadTestAttachment(t *testing.T,s *Server,token,room,name string, content []byte)*httptest.ResponseRecorder{
 t.Helper()
 var body bytes.Buffer
 writer:=multipart.NewWriter(&body)
 if err:=writer.WriteField("roomId",room);err!=nil{t.Fatal(err)}
 file,err:=writer.CreateFormFile("file",name)
 if err!=nil{t.Fatal(err)}
 if _,err=file.Write(content);err!=nil{t.Fatal(err)}
 if err:=writer.Close();err!=nil{t.Fatal(err)}
 req:=httptest.NewRequest("POST","/api/files/upload",&body)
 req.Header.Set("Authorization","Bearer "+token)
 req.Header.Set("Content-Type",writer.FormDataContentType())
 result:=httptest.NewRecorder()
 s.Handler().ServeHTTP(result,req)
 return result
}
func TestSecureFileUploadAndRoomDownload(t *testing.T){
 s:=testServer(t)
 alice:=reg(t,s,"alice")
 bob:=reg(t,s,"bob")
 carol:=reg(t,s,"carol")
 room,err:=s.store.CreateGroup("alice","研发群",[]string{"bob"})
 if err!=nil{t.Fatal(err)}
 data:=[]byte("report contents\n12345")
 result:=uploadTestAttachment(t,s,alice,room.ID,"../../report.txt",data)
 if result.Code!=201{t.Fatalf("upload: %d %s",result.Code,result.Body.String())}
 var message ChatMessage
 if err:=json.Unmarshal(result.Body.Bytes(),&message);err!=nil{t.Fatal(err)}
 if message.Attachment==nil || message.Attachment.Name!="report.txt" ||
  message.Attachment.Size!=int64(len(data)) ||
  message.Text!="[文件] report.txt"{t.Fatalf("attachment record: %+v",message)}
 id:=message.Attachment.ID
 if len(id)!=32{t.Fatal("invalid attachment id")}
 got:=s.store.PendingFor("bob")
 if len(got)!=1||got[0].Attachment==nil||got[0].Attachment.ID!=id{t.Fatal("member did not receive file message",got)}
 root:=filepath.Join(filepath.Dir(s.store.path),"uploads",id+".blob")
 stored,err:=os.ReadFile(root)
 if err!=nil||!bytes.Equal(stored,data){t.Fatal("disk attachment contents",err)}
 reloaded,err:=NewStore(filepath.Dir(s.store.path))
 if err!=nil{t.Fatal(err)}
 if _,ok:=reloaded.attachmentForUser(id,"bob");!ok{t.Fatal("file metadata did not persist")}
 if _,ok:=reloaded.attachmentForUser(id,"carol");ok{t.Fatal("file leaked to nonmember")}

 for _,test:=range []struct{token string;expected int}{
    {bob,200},{alice,200},{carol,404},{"",401},
 } {
    req:=httptest.NewRequest("GET","/api/files/"+id,nil)
    if test.token!=""{req.Header.Set("Authorization","Bearer "+test.token)}
    response:=httptest.NewRecorder()
    s.Handler().ServeHTTP(response,req)
    if response.Code!=test.expected {t.Errorf("download returned %d instead of %d",response.Code,test.expected)}
    if response.Code==200{
      if !bytes.Equal(response.Body.Bytes(),data){t.Fatal("downloaded bytes corrupted")}
      if !strings.HasPrefix(response.Header().Get("Content-Disposition"),"attachment;"){t.Fatal("file not forced to download")}
    }
 }
 forbidden:=uploadTestAttachment(t,s,carol,room.ID,"secret.txt",data)
 if forbidden.Code!=403{t.Fatalf("nonmember could upload to group: %d",forbidden.Code)}
 empty:=uploadTestAttachment(t,s,alice,room.ID,"empty.txt",nil)
 if empty.Code!=400{t.Fatalf("empty file accepted: %d",empty.Code)}
 tooLarge:=uploadTestAttachment(t,s,alice,room.ID,"huge.dat",bytes.Repeat([]byte("a"),int(maxUploadBytes)+1))
 if tooLarge.Code!=413{t.Fatalf("oversize file accepted: %d %s",tooLarge.Code,tooLarge.Body.String())}
}
func TestGroupCreateAppearsToOfflineMember(t *testing.T){
 s:=testServer(t)
 alice:=reg(t,s,"alice")
 bob:=reg(t,s,"bob")
 carol:=reg(t,s,"carol")
 created:=authenticatedJSON(t,s,"POST","/api/rooms",alice,
   map[string]any{"name":"技术群","members":[]string{"bob"}})
 if created.Code!=201{t.Fatalf("group creation failed: %d %s",created.Code,created.Body.String())}
 var room Room
 if err:=json.Unmarshal(created.Body.Bytes(),&room);err!=nil{t.Fatal(err)}
 if room.Type!="group"||!member(room,"alice")||!member(room,"bob")||member(room,"carol"){
   t.Fatal("bad group membership",room)
 }
 pending:=authenticatedJSON(t,s,"GET","/api/messages/pending",bob,nil)
 var response struct{Rooms []Room `json:"rooms"`}
 if err:=json.Unmarshal(pending.Body.Bytes(),&response);err!=nil{t.Fatal(err)}
 exists:=false
 for _,r:=range response.Rooms {if r.ID==room.ID{exists=true}}
 if !exists{t.Fatal("newly created group not discoverable via HTTP fallback")}
 denied:=authenticatedJSON(t,s,"GET","/api/messages/pending",carol,nil)
 var outsider struct{Rooms []Room `json:"rooms"`}
 if err:=json.Unmarshal(denied.Body.Bytes(),&outsider);err!=nil{t.Fatal(err)}
 for _,r:=range outsider.Rooms {if r.ID==room.ID{t.Fatal("leaked private group")}}
 sent:=authenticatedJSON(t,s,"POST","/api/messages/send",alice,
   map[string]string{"roomId":room.ID,"text":"技术群新消息"})
 if sent.Code!=201{t.Fatal("member could not send to group")}
 if got:=s.store.PendingFor("bob");len(got)!=1||got[0].Text!="技术群新消息"{t.Fatal("group member did not receive pending message",got)}
}
// Compile-time check to ensure uploaded content is served as an io stream.
var _ io.Reader

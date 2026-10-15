package chat

import (
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "log"
    "net/http"
    "net/url"
    "os"
    "path/filepath"
    "strings"
    "unicode"
    "unicode/utf8"
)

const maxUploadBytes int64 = 10 << 20
const maxStoredAttachments = 2000

// The attachment metadata and reference are persisted in state.json alongside
// the durable pending message. The bytes live only under DATA_DIR/uploads.
type Attachment struct {
    ID string `json:"id"`
    Name string `json:"name"`
    Size int64 `json:"size"`
    MIME string `json:"mime"`
    URL string `json:"url"`
}
type StoredAttachment struct {
    Attachment Attachment `json:"attachment"`
    RoomID string `json:"roomId"`
    Sender string `json:"sender"`
}

func (s *Store) attachmentForUser(id,user string)(Attachment,bool){
    s.mu.RLock()
    defer s.mu.RUnlock()
    record,ok:=s.state.Attachments[id]
    if !ok{return Attachment{},false}
    room,found:=s.state.Rooms[record.RoomID]
    if !found||!member(room,user){return Attachment{},false}
    return record.Attachment,true
}
func (s *Store) attachmentCapacity()bool{
    s.mu.RLock()
    defer s.mu.RUnlock()
    return len(s.state.Attachments)<maxStoredAttachments
}
func cleanFilename(input string)string{
    name:=filepath.Base(strings.ReplaceAll(input,"\\","/"))
    var result []rune
    for _,c:=range name{
        if !unicode.IsControl(c) && c!='/' && c!='\\'{
            result=append(result,c)
            if len(result)==120{break}
        }
    }
    out:=strings.TrimSpace(string(result))
    if out==""||out=="."||out==".."{return "attachment"}
    return out
}
func (s *Server) uploadFile(w http.ResponseWriter,r *http.Request,user string){
    if s.limitSend(user){apiError(w,429,"文件上传太频繁");return}
    // The extra 1MB is multipart overhead; the actual file cap is 10MB.
    r.Body=http.MaxBytesReader(w,r.Body,maxUploadBytes+(1<<20))
    if err:=r.ParseMultipartForm(1<<20);err!=nil{
        apiError(w,413,"文件超过 10MB 或上传格式无效")
        return
    }
    if r.MultipartForm!=nil{defer r.MultipartForm.RemoveAll()}
    roomID:=r.FormValue("roomId")
    if _,exists:=s.store.RoomForUser(roomID,user);!exists{
        apiError(w,403,"无权向该会话上传文件")
        return
    }
    if !s.store.attachmentCapacity(){apiError(w,507,"服务器附件容量已满");return}
    source,header,err:=r.FormFile("file")
    if err!=nil{apiError(w,400,"请选择上传文件");return}
    defer source.Close()
    if header.Size>maxUploadBytes{apiError(w,413,"文件不能超过 10MB");return}

    dir:=filepath.Join(filepath.Dir(s.store.path),"uploads")
    if err:=os.MkdirAll(dir,0700);err!=nil{log.Printf("upload dir: %v",err);apiError(w,500,"无法创建附件目录");return}
    id,err:=randomID()
    if err!=nil{apiError(w,500,"无法分配附件编号");return}
    // Untrusted filenames are metadata only, never used as filesystem paths.
    temp,err:=os.CreateTemp(dir,".upload-*")
    if err!=nil{apiError(w,500,"无法创建附件文件");return}
    tempName:=temp.Name()
    defer os.Remove(tempName)
    if err:=temp.Chmod(0600);err!=nil{temp.Close();apiError(w,500,"无法限制附件权限");return}
    total,err:=io.Copy(temp,io.LimitReader(source,maxUploadBytes+1))
    if err!=nil||total>maxUploadBytes{temp.Close();apiError(w,413,"附件上传失败或超出10MB");return}
    if total==0{temp.Close();apiError(w,400,"空文件不可上传");return}
    headerBytes:=make([]byte,512)
    count,err:=temp.ReadAt(headerBytes,0)
    if err!=nil && !errors.Is(err,io.EOF){temp.Close();apiError(w,500,"读取附件失败");return}
    detected:=http.DetectContentType(headerBytes[:count])
    if err:=temp.Sync();err!=nil{temp.Close();apiError(w,500,"保存附件失败");return}
    if err:=temp.Close();err!=nil{apiError(w,500,"保存附件失败");return}
    finalPath:=filepath.Join(dir,id+".blob")
    if err:=os.Rename(tempName,finalPath);err!=nil{apiError(w,500,"保存附件失败");return}

    attachment:=Attachment{ID:id,Name:cleanFilename(header.Filename),Size:total,
        MIME:detected,URL:"/api/files/"+id}
    text:="[文件] "+attachment.Name
    if utf8.RuneCountInString(text)>4000{text="[文件]"}
    room,msg,err:=s.store.QueueAttachmentMessage(roomID,user,text,&attachment)
    if err!=nil{
        _=os.Remove(finalPath)
        if errors.Is(err,ErrInvalid){apiError(w,403,"无权向该会话上传文件");return}
        log.Printf("upload message commit failed: %v",err)
        apiError(w,500,"保存附件消息失败")
        return
    }
    data,err:=json.Marshal(msg)
    if err==nil{s.hub.broadcast(room,data)}
    s.queuePush(room,msg)
    s.queueNativePush(room,user)
    writeJSON(w,http.StatusCreated,msg)
}
func (s *Server) downloadFile(w http.ResponseWriter,r *http.Request,user string){
    id:=r.PathValue("id")
    if !validDeviceID(id){http.NotFound(w,r);return}
    item,ok:=s.store.attachmentForUser(id,user)
    if !ok{http.NotFound(w,r);return}
    file,err:=os.Open(filepath.Join(filepath.Dir(s.store.path),"uploads",id+".blob"))
    if err!=nil{http.NotFound(w,r);return}
    defer file.Close()
    info,err:=file.Stat()
    if err!=nil||!info.Mode().IsRegular(){http.NotFound(w,r);return}
    w.Header().Set("Content-Type","application/octet-stream")
    w.Header().Set("X-Content-Type-Options","nosniff")
    w.Header().Set("Content-Disposition",fmt.Sprintf("attachment; filename=\"download\"; filename*=UTF-8''%s",url.PathEscape(item.Name)))
    w.Header().Set("Content-Length",fmt.Sprintf("%d",info.Size()))
    _,_=io.Copy(w,file)
}

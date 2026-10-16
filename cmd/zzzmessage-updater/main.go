package main

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net"
 "net/http"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "time"
)

type updateResult struct {
 Tag string `json:"tag"`
 Success bool `json:"success"`
 Message string `json:"message"`
 Time string `json:"time"`
}
func record(data,tag string,success bool,msg string){
 if len(msg)>250{msg=msg[:250]}
 result:=updateResult{Tag:tag,Success:success,Message:msg,Time:time.Now().UTC().Format(time.RFC3339)}
 raw,_:=json.MarshalIndent(result,"","  ")
 tmp,err:=os.CreateTemp(data,".update-result-*")
 if err!=nil{return}
 defer os.Remove(tmp.Name())
 _=tmp.Chmod(0600)
 _,_=tmp.Write(raw)
 if tmp.Close()!=nil{return}
 _=os.Rename(tmp.Name(),filepath.Join(data,"update-result.json"))
}
func copyFile(source,dest string)error{
 input,err:=os.Open(source);if err!=nil{return err}
 defer input.Close()
 info,err:=input.Stat();if err!=nil{return err}
 target,err:=os.OpenFile(dest,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600);if err!=nil{return err}
 _,copyErr:=io.Copy(target,input)
 closeErr:=target.Close()
 if copyErr!=nil{return copyErr}
 if closeErr!=nil{return closeErr}
 if info.Size()<0{return errors.New("invalid data backup")}
 return nil
}
func healthURL()string{
 address:=os.Getenv("LISTEN_ADDR")
 if address==""{address="127.0.0.1:58463"}
 host,port,err:=net.SplitHostPort(address)
 if err!=nil||port==""{return "http://127.0.0.1:58463/api/health"}
 if host==""||host=="0.0.0.0"||host=="::"{host="127.0.0.1"}
 if host=="[::1]"{host="::1"}
 return "http://"+net.JoinHostPort(host,port)+"/api/health"
}
func waitHealth(ctx context.Context)error{
 client:=&http.Client{Timeout:1200*time.Millisecond}
 url:=healthURL()
 timer:=time.NewTicker(500*time.Millisecond)
 defer timer.Stop()
 for{
  req,err:=http.NewRequestWithContext(ctx,"GET",url,nil)
  if err!=nil{return err}
  reply,err:=client.Do(req)
  if err==nil{
   var response struct{App string `json:"app"`}
   _=json.NewDecoder(io.LimitReader(reply.Body,4096)).Decode(&response)
   _=reply.Body.Close()
   if reply.StatusCode==200&&response.App=="ZzzMessage"{return nil}
  }
  select{
  case <-ctx.Done():return errors.New("restarted server health check timed out")
  case <-timer.C:
  }
 }
}
func startBackend(root string)(*exec.Cmd,error){
 cmd:=exec.Command(filepath.Join(root,"ZzzMessage.exe"))
 cmd.Dir=root
 cmd.Env=os.Environ()
 cmd.Stdin=nil;cmd.Stdout=nil;cmd.Stderr=nil
 if err:=cmd.Start();err!=nil{return nil,err}
 _=cmd.Process.Release()
 return cmd,nil
}
func applyUpdate(stage,root,data,tag string)error{
 // The updater never accepts arbitrary URLs, scripts or command arguments
 // from the browser. Paths are derived from the server installation.
 exe:=filepath.Join(root,"ZzzMessage.exe")
 web:=filepath.Join(root,"web")
 nextExe:=filepath.Join(stage,"ZzzMessage.exe")
 nextWeb:=filepath.Join(stage,"web")
 if _,err:=os.Stat(nextExe);err!=nil{return fmt.Errorf("staged backend unavailable: %w",err)}
 if _,err:=os.Stat(filepath.Join(nextWeb,"index.html"));err!=nil{return fmt.Errorf("staged UI unavailable: %w",err)}
 id:=time.Now().UTC().Format("20060102-150405")
 backup:=filepath.Join(data,"updates","backups",id)
 if err:=os.MkdirAll(backup,0700);err!=nil{return err}
 if err:=copyFile(filepath.Join(data,"state.json"),filepath.Join(backup,"state.json"));err!=nil&&!errors.Is(err,os.ErrNotExist){
  return fmt.Errorf("state backup failed: %w",err)
 }
 oldExe:=filepath.Join(backup,"ZzzMessage.exe")
 oldWeb:=filepath.Join(backup,"web")
 // On Windows, rename of a running .exe fails; wait for API server shutdown.
 deadline:=time.Now().Add(40*time.Second)
 for {
  err:=os.Rename(exe,oldExe)
  if err==nil{break}
  if time.Now().After(deadline){return fmt.Errorf("old server still running or backend cannot be replaced: %w",err)}
  time.Sleep(300*time.Millisecond)
 }
 hasExe:=true
 hasWeb:=false
 newExe:=false
 newWeb:=false
 rollback:=func(){
  if newWeb {_=os.RemoveAll(web)}
  if newExe {_=os.Remove(exe)}
  if hasWeb {_=os.Rename(oldWeb,web)}
  if hasExe {_=os.Rename(oldExe,exe)}
 }
 if err:=os.Rename(web,oldWeb);err!=nil{rollback();return fmt.Errorf("frontend backup failed: %w",err)}
 hasWeb=true
 if err:=os.Rename(nextExe,exe);err!=nil{rollback();return fmt.Errorf("backend install failed: %w",err)}
 newExe=true
 if err:=os.Rename(nextWeb,web);err!=nil{rollback();return fmt.Errorf("frontend install failed: %w",err)}
 newWeb=true
 cmd,err:=startBackend(root)
 if err!=nil{rollback();_,_ =startBackend(root);return fmt.Errorf("failed to launch updated backend: %w",err)}
 ctx,cancel:=context.WithTimeout(context.Background(),25*time.Second)
 defer cancel()
 if err:=waitHealth(ctx);err!=nil{
  _=cmd.Process.Kill()
  time.Sleep(1100*time.Millisecond)
  rollback()
  _,_ =startBackend(root)
  return fmt.Errorf("updated backend failed health check; previous binary and frontend restored: %w",err)
 }
 _=os.RemoveAll(filepath.Dir(stage))
 return nil
}
func main(){
 if len(os.Args)!=6||os.Args[1]!="--apply"{
  fmt.Fprintln(os.Stderr,"ZzzMessage-Updater only runs when launched by the admin backend")
  os.Exit(2)
 }
 stage,root,data,tag:=os.Args[2],os.Args[3],os.Args[4],os.Args[5]
 if !filepath.IsAbs(stage)||!filepath.IsAbs(root)||!filepath.IsAbs(data)||
  !strings.HasPrefix(filepath.Clean(stage)+string(os.PathSeparator),filepath.Join(data,"updates")+string(os.PathSeparator))||
  len(tag)>64 {
  fmt.Fprintln(os.Stderr,"invalid updater arguments")
  os.Exit(2)
 }
 err:=applyUpdate(stage,root,data,tag)
 if err!=nil{
  record(data,tag,false,err.Error())
  // On failures after server shutdown, ensure that the restored binary is
  // launched. Do not start a duplicate if the old server is still healthy.
  probeCtx,cancel:=context.WithTimeout(context.Background(),2*time.Second)
  existing:=waitHealth(probeCtx)
  cancel()
  if existing!=nil {
    _,_ =startBackend(root)
  }
  os.Exit(1)
 }
 record(data,tag,true,"新版本已安装并通过健康检查；旧后端、旧前端和状态快照保留在 data/updates/backups")
}

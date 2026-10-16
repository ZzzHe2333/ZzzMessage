package chat

import (
 "archive/zip"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "net/url"
 "os"
 "os/exec"
 "path"
 "path/filepath"
 "runtime"
 "strings"
 "sync"
 "time"
)

const updateRepo = "ZzzHe2333/ZzzMessage"
const releaseAPI = "https://api.github.com/repos/"+updateRepo+"/releases?per_page=12"
const maxArchiveSize int64 = 160 << 20

type ReleaseInfo struct {
 Tag string `json:"tag"`
 PublishedAt string `json:"publishedAt"`
 Prerelease bool `json:"prerelease"`
 URL string `json:"url"`
}
type officialAsset struct {
 Name string `json:"name"`
 DownloadURL string `json:"browser_download_url"`
 Size int64 `json:"size"`
}
type officialRelease struct {
 Tag string `json:"tag_name"`
 Draft bool `json:"draft"`
 Prerelease bool `json:"prerelease"`
 PublishedAt string `json:"published_at"`
 HTMLURL string `json:"html_url"`
 Assets []officialAsset `json:"assets"`
}
type AdminUpdater struct {
 owner string
 installDir string
 dataDir string
 buildTime string
 version string
 mu sync.Mutex
 busy bool
 shutdown func()
 client *http.Client
}
func NewAdminUpdater(owner,installDir,dataDir,version,buildTime string)(*AdminUpdater,error){
 if owner=="" {return nil,nil}
 if runtime.GOOS!="windows" { /* admin dashboard still available but applying is unsupported */ }
 root,err:=filepath.Abs(installDir);if err!=nil{return nil,err}
 data,err:=filepath.Abs(dataDir);if err!=nil{return nil,err}
 if owner!=strings.ToLower(owner)||!validUsername(owner){return nil,errors.New("ADMIN_USERNAME 必须是已注册的小写用户名")}
 client:=&http.Client{Timeout:45*time.Second,CheckRedirect:func(req *http.Request,via []*http.Request)error{
  if len(via)>5 || req.URL.Scheme!="https"{return errors.New("blocked unsafe update redirect")}
  switch strings.ToLower(req.URL.Hostname()){
  case "api.github.com","github.com","release-assets.githubusercontent.com","objects.githubusercontent.com":
   return nil
  }
  return errors.New("blocked update redirect to untrusted host")
 }}
 return &AdminUpdater{owner:owner,installDir:root,dataDir:data,version:version,buildTime:buildTime,client:client},nil
}
func (s *Server) SetAdminUpdater(updater *AdminUpdater){
 s.adminUpdater=updater
}
func (s *Server) SetAdminShutdown(shutdown func()){
 if s.adminUpdater!=nil{s.adminUpdater.shutdown=shutdown}
}
func (s *Server) isAdmin(user string)bool{
 return s.adminUpdater!=nil && user==s.adminUpdater.owner
}
func (s *Server) adminMe(w http.ResponseWriter,r *http.Request,user string){
 writeJSON(w,200,map[string]any{"admin":s.isAdmin(user)})
}
func (s *Server) adminGuard(w http.ResponseWriter,user string)bool{
 if !s.isAdmin(user){apiError(w,403,"仅服务器管理员可操作");return false}
 return true
}
func (a *AdminUpdater) supported()bool{
 _,err:=os.Stat(filepath.Join(a.installDir,"ZzzMessage.exe"))
 if err!=nil{return false}
 _,err=os.Stat(filepath.Join(a.installDir,"ZzzMessage-Updater.exe"))
 return runtime.GOOS=="windows"&&runtime.GOARCH=="amd64"&&err==nil
}
func (s *Server) adminStatus(w http.ResponseWriter,r *http.Request,user string){
 if !s.adminGuard(w,user){return}
 a:=s.adminUpdater
 a.mu.Lock();busy:=a.busy;a.mu.Unlock()
 result:=map[string]any{
  "enabled":true,"supported":a.supported(),"platform":runtime.GOOS+"/"+runtime.GOARCH,
  "installedVersion":a.version,"buildTime":a.buildTime,"busy":busy,
  "updateSource":"https://github.com/"+updateRepo+"/releases",
 }
 // Only show persisted result metadata. Never expose filesystem paths.
 data,err:=os.ReadFile(filepath.Join(a.dataDir,"update-result.json"))
 if err==nil{
  var previous map[string]any
  if json.Unmarshal(data,&previous)==nil{result["lastResult"]=previous}
 }
 writeJSON(w,200,result)
}
func (a *AdminUpdater) officialReleases(ctx context.Context)([]officialRelease,error){
 req,err:=http.NewRequestWithContext(ctx,"GET",releaseAPI,nil)
 if err!=nil{return nil,err}
 req.Header.Set("User-Agent","ZzzMessage-admin-updater/1")
 resp,err:=a.client.Do(req)
 if err!=nil{return nil,err}
 defer resp.Body.Close()
 if resp.StatusCode!=200{return nil,fmt.Errorf("GitHub Release API: HTTP %d",resp.StatusCode)}
 var list []officialRelease
 if err:=json.NewDecoder(io.LimitReader(resp.Body,2<<20)).Decode(&list);err!=nil{return nil,err}
 return list,nil
}
func releaseAssets(rel officialRelease)(officialAsset,officialAsset,bool){
 var archive,sums officialAsset
 for _,asset:=range rel.Assets{
  switch asset.Name{
  case "ZzzMessage-Windows-amd64.zip": archive=asset
  case "SHA256SUMS.txt": sums=asset
  }
 }
 return archive,sums,archive.DownloadURL!=""&&sums.DownloadURL!=""
}
func toReleaseInfo(r officialRelease)ReleaseInfo{
 return ReleaseInfo{Tag:r.Tag,PublishedAt:r.PublishedAt,Prerelease:r.Prerelease,URL:r.HTMLURL}
}
func (s *Server) adminReleases(w http.ResponseWriter,r *http.Request,user string){
 if !s.adminGuard(w,user){return}
 ctx,cancel:=context.WithTimeout(r.Context(),20*time.Second);defer cancel()
 list,err:=s.adminUpdater.officialReleases(ctx)
 if err!=nil{apiError(w,502,"读取 GitHub Release 失败："+err.Error());return}
 out:=make([]ReleaseInfo,0,len(list))
 for _,rel:=range list{
  if rel.Draft||!validTag(rel.Tag){continue}
  _,_,ok:=releaseAssets(rel)
  if ok{out=append(out,toReleaseInfo(rel))}
 }
 writeJSON(w,200,map[string]any{"releases":out})
}
func validTag(tag string)bool{
 if len(tag)<3||len(tag)>64||tag[0]!='v'{return false}
 for _,c:=range tag[1:]{
  if !(c>='0'&&c<='9'||c>='a'&&c<='z'||c=='.'||c=='-'){return false}
 }
 return true
}
func verifiedAssetURL(asset officialAsset,tag string)bool{
 u,err:=url.Parse(asset.DownloadURL)
 if err!=nil{return false}
 return u.Scheme=="https"&&u.Host=="github.com"&&u.User==nil&&
  u.RawQuery==""&&u.Fragment==""&&
  u.Path=="/"+updateRepo+"/releases/download/"+tag+"/"+asset.Name
}
func (a *AdminUpdater) download(ctx context.Context,asset officialAsset,dest string,limit int64)error{
 req,err:=http.NewRequestWithContext(ctx,"GET",asset.DownloadURL,nil)
 if err!=nil{return err}
 req.Header.Set("User-Agent","ZzzMessage-admin-updater/1")
 resp,err:=a.client.Do(req);if err!=nil{return err}
 defer resp.Body.Close()
 if resp.StatusCode!=200{return fmt.Errorf("GitHub 下载失败: HTTP %d",resp.StatusCode)}
 if resp.ContentLength>limit{return errors.New("下载文件超过安全大小限制")}
 file,err:=os.OpenFile(dest,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600)
 if err!=nil{return err}
 copied,copyErr:=io.Copy(file,io.LimitReader(resp.Body,limit+1))
 closeErr:=file.Close()
 if copyErr!=nil{return copyErr}
 if closeErr!=nil{return closeErr}
 if copied>limit{return errors.New("下载超出安全大小限制")}
 if asset.Size>0&&copied!=asset.Size{return errors.New("下载文件与 GitHub 声明大小不一致")}
 return nil
}
func sha256File(file string)(string,error){
 reader,err:=os.Open(file);if err!=nil{return "",err};defer reader.Close()
 hasher:=sha256.New()
 if _,err:=io.Copy(hasher,reader);err!=nil{return "",err}
 return hex.EncodeToString(hasher.Sum(nil)),nil
}
func verifySHA256Sums(raw []byte,filename,want string)bool{
 for _,line:=range strings.Split(string(raw),"\n"){
  fields:=strings.Fields(line)
  if len(fields)==2&&strings.TrimPrefix(fields[1],"*")==filename&&
   len(fields[0])==64&&strings.EqualFold(fields[0],want){return true}
 }
 return false
}
func safeWindowsArchive(zipPath,stageDir string)error{
 reader,err:=zip.OpenReader(zipPath);if err!=nil{return err}
 defer reader.Close()
 if len(reader.File)>2000{return errors.New("更新包文件过多")}
 prefix:="ZzzMessage-windows-amd64/"
 var extracted int64
 seen:=map[string]bool{}
 for _,entry:=range reader.File{
  name:=entry.Name
  if !strings.HasPrefix(name,prefix){continue}
  relative:=strings.TrimPrefix(name,prefix)
  if relative==""||strings.HasSuffix(relative,"/"){continue}
  clean:=path.Clean(relative)
  if clean!=relative||strings.Contains(clean,"\\")||strings.HasPrefix(clean,"../")||strings.Contains(clean,":")||
   entry.Mode()&os.ModeSymlink!=0{return errors.New("更新包存在不安全的文件路径")}
  if clean!="ZzzMessage.exe"&&!strings.HasPrefix(clean,"web/"){continue}
  if seen[clean]{return errors.New("更新包存在重复文件")}
  seen[clean]=true
  if entry.UncompressedSize64>80<<20{return errors.New("更新包单文件过大")}
  dest:=filepath.Join(stageDir,filepath.FromSlash(clean))
  if err:=os.MkdirAll(filepath.Dir(dest),0700);err!=nil{return err}
  input,err:=entry.Open();if err!=nil{return err}
  output,err:=os.OpenFile(dest,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600)
  if err!=nil{input.Close();return err}
  count,copyErr:=io.Copy(output,io.LimitReader(input,80<<20+1))
  closeErr:=output.Close();_ =input.Close()
  if copyErr!=nil{return copyErr}
  if closeErr!=nil{return closeErr}
  extracted+=count
  if count>80<<20||extracted>maxArchiveSize{return errors.New("更新包解压大小超限")}
 }
 if !seen["ZzzMessage.exe"]||!seen["web/index.html"]{return errors.New("更新包缺少后端或前端")}
 // No remote scripts or environment files are written to the target.
 f,err:=os.Open(filepath.Join(stageDir,"ZzzMessage.exe"));if err!=nil{return err}
 defer f.Close()
 magic:=make([]byte,2);_,err=io.ReadFull(f,magic)
 if err!=nil||string(magic)!="MZ"{return errors.New("后端文件不是 Windows PE 可执行程序")}
 return nil
}
func (a *AdminUpdater) stage(ctx context.Context,tag string)(string,error){
 list,err:=a.officialReleases(ctx);if err!=nil{return "",err}
 var release *officialRelease
 for i:=range list{if list[i].Tag==tag&&!list[i].Draft{release=&list[i];break}}
 if release==nil{return "",errors.New("未找到指定已发布版本")}
 archive,sums,ok:=releaseAssets(*release)
 if !ok||!verifiedAssetURL(archive,tag)||!verifiedAssetURL(sums,tag){
  return "",errors.New("官方版本缺少可信的 Windows 安装包或校验文件")
 }
 if archive.Size<=0||archive.Size>maxArchiveSize||sums.Size>128<<10{
  return "",errors.New("官方更新包大小不合法")
 }
 if a.buildTime!=""{
  build,err1:=time.Parse(time.RFC3339,a.buildTime)
  published,err2:=time.Parse(time.RFC3339,release.PublishedAt)
  if err1==nil&&err2==nil&&!published.After(build){
   return "",errors.New("该 Release 早于当前程序的构建时间，禁止降级覆盖")
  }
 }
 root:=filepath.Join(a.dataDir,"updates")
 if err:=os.MkdirAll(root,0700);err!=nil{return "",err}
 stage,err:=os.MkdirTemp(root,"stage-*");if err!=nil{return "",err}
 success:=false
 defer func(){if !success{os.RemoveAll(stage)}}()
 rawZip:=filepath.Join(stage,"package.zip")
 if err:=a.download(ctx,archive,rawZip,maxArchiveSize);err!=nil{return "",err}
 sumsFile:=filepath.Join(stage,"SHA256SUMS.txt")
 if err:=a.download(ctx,sums,sumsFile,128<<10);err!=nil{return "",err}
 hash,err:=sha256File(rawZip);if err!=nil{return "",err}
 data,err:=os.ReadFile(sumsFile);if err!=nil{return "",err}
 if !verifySHA256Sums(data,archive.Name,hash){return "",errors.New("下载的安装包 SHA-256 校验失败")}
 payload:=filepath.Join(stage,"payload")
 if err:=safeWindowsArchive(rawZip,payload);err!=nil{return "",err}
 // Keep the staged payload only; never move or overwrite .env / data during install.
 _=os.Remove(rawZip);_=os.Remove(sumsFile)
 success=true
 return payload,nil
}
func (s *Server) adminApply(w http.ResponseWriter,r *http.Request,user string){
 if !s.adminGuard(w,user){return}
 a:=s.adminUpdater
 if !a.supported(){apiError(w,409,"当前不是受支持的 Windows 原生部署或缺少 ZzzMessage-Updater.exe");return}
 if a.shutdown==nil{apiError(w,503,"服务端尚未配置安全重启管理");return}
 var input struct{Tag string `json:"tag"`;Confirmation string `json:"confirmation"`}
 if !readJSON(w,r,&input){return}
 if !validTag(input.Tag)||input.Confirmation!="UPDATE "+input.Tag{
  apiError(w,400,"版本或确认短语错误");return
 }
 a.mu.Lock()
 if a.busy{a.mu.Unlock();apiError(w,409,"已有更新任务运行中");return}
 a.busy=true
 a.mu.Unlock()
 started:=false
 defer func(){
  if !started{a.mu.Lock();a.busy=false;a.mu.Unlock()}
 }()
 ctx,cancel:=context.WithTimeout(r.Context(),2*time.Minute)
 defer cancel()
 stage,err:=a.stage(ctx,input.Tag)
 if err!=nil{apiError(w,502,"下载或验证更新包失败："+err.Error());return}
 exe:=filepath.Join(a.installDir,"ZzzMessage-Updater.exe")
 cmd:=exec.Command(exe,"--apply",stage,a.installDir,a.dataDir,input.Tag)
 cmd.Dir=a.installDir
 // A detached helper waits for this Go server to release its executable.
 // It never inherits the HTTP connection or sends secrets to stdout.
 cmd.Stdin=nil;cmd.Stdout=nil;cmd.Stderr=nil
 if err:=cmd.Start();err!=nil{
  _=os.RemoveAll(filepath.Dir(stage))
  apiError(w,500,"无法启动更新辅助程序："+err.Error());return
 }
 _=cmd.Process.Release()
 started=true
 writeJSON(w,202,map[string]any{"accepted":true,"tag":input.Tag,"message":"已校验官方更新包，服务即将重启"})
 go func(){time.Sleep(850*time.Millisecond);a.shutdown()}()
}

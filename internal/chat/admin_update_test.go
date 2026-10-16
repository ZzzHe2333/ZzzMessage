package chat

import (
 "archive/zip"
 "bytes"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestAdminAccountRoleAndUnauthorizedUpdate(t *testing.T){
 s:=testServer(t)
 alice:=reg(t,s,"alice")
 bob:=reg(t,s,"bob")
 resp:=authenticatedJSON(t,s,"GET","/api/admin/me",alice,nil)
 if resp.Code!=200||!strings.Contains(resp.Body.String(),"false"){t.Fatal("admin enabled before configuring")}
 updater,err:=NewAdminUpdater("alice",t.TempDir(),t.TempDir(),"dev","2026-10-16T00:00:00Z")
 if err!=nil{t.Fatal(err)}
 s.SetAdminUpdater(updater)
 resp=authenticatedJSON(t,s,"GET","/api/admin/me",alice,nil)
 if resp.Code!=200||!strings.Contains(resp.Body.String(),"true"){t.Fatal("owner not recognized")}
 for _,path:=range []string{"/api/admin/status","/api/admin/releases"}{
  got:=authenticatedJSON(t,s,"GET",path,bob,nil)
  if got.Code!=403{t.Fatalf("nonadmin used %s: %d",path,got.Code)}
 }
 denied:=authenticatedJSON(t,s,"POST","/api/admin/update",bob,
  map[string]string{"tag":"v0.4.0","confirmation":"UPDATE v0.4.0"})
 if denied.Code!=403{t.Fatal("nonadmin initiated update")}
 unauth:=httptest.NewRecorder()
 s.Handler().ServeHTTP(unauth,httptest.NewRequest("GET","/api/admin/status",nil))
 if unauth.Code!=401{t.Fatal("anonymous admin status")}
 if got:=authenticatedJSON(t,s,"GET","/api/admin/status",alice,nil);got.Code!=200{
  t.Fatalf("admin status failed: %s",got.Body.String())
 }
}
func TestAdminReleaseURLAndChecksumSafety(t *testing.T){
 if !validTag("v0.4.0-beta.1")||validTag("v../bad")||validTag("../evil"){t.Fatal("invalid release tag validation")}
 asset:=officialAsset{Name:"ZzzMessage-Windows-amd64.zip",DownloadURL:"https://github.com/ZzzHe2333/ZzzMessage/releases/download/v0.4.0/ZzzMessage-Windows-amd64.zip"}
 if !verifiedAssetURL(asset,"v0.4.0"){t.Fatal("valid release asset rejected")}
 for _,value:=range []string{
  "https://github.com.evil.org/ZzzHe2333/ZzzMessage/releases/download/v0.4.0/ZzzMessage-Windows-amd64.zip",
  "https://github.com/ZzzHe2333/ZzzMessage/releases/download/v0.4.0/other.zip",
  "http://github.com/ZzzHe2333/ZzzMessage/releases/download/v0.4.0/ZzzMessage-Windows-amd64.zip",
 }{
  asset.DownloadURL=value
  if verifiedAssetURL(asset,"v0.4.0"){t.Fatal("untrusted asset URL accepted")}
 }
 hash:=strings.Repeat("a",64)
 sums:=[]byte(hash+"  ZzzMessage-Windows-amd64.zip\n")
 if !verifySHA256Sums(sums,"ZzzMessage-Windows-amd64.zip",hash){t.Fatal("checksum mismatch")}
 if verifySHA256Sums(sums,"other.zip",hash){t.Fatal("checksum wrong filename accepted")}
}
func createZIP(t *testing.T,files map[string]string)string{
 t.Helper()
 var buffer bytes.Buffer
 w:=zip.NewWriter(&buffer)
 for name,body:=range files{
  entry,err:=w.Create(name);if err!=nil{t.Fatal(err)}
  if _,err:=entry.Write([]byte(body));err!=nil{t.Fatal(err)}
 }
 if err:=w.Close();err!=nil{t.Fatal(err)}
 file:=filepath.Join(t.TempDir(),"update.zip")
 if err:=os.WriteFile(file,buffer.Bytes(),0600);err!=nil{t.Fatal(err)}
 return file
}
func TestAdminArchivePreventsDataOverwriteAndPathTraversal(t *testing.T){
 prefix:="ZzzMessage-windows-amd64/"
 good:=createZIP(t,map[string]string{
  prefix+"ZzzMessage.exe":"MZ0000",
  prefix+"web/index.html":"new ui",
  prefix+".env":"BAD=1",
  prefix+"data/state.json":"not your data",
 })
 dir:=t.TempDir()
 if err:=safeWindowsArchive(good,dir);err!=nil{t.Fatal(err)}
 if _,err:=os.Stat(filepath.Join(dir,".env"));!os.IsNotExist(err){t.Fatal("archive modified .env")}
 if _,err:=os.Stat(filepath.Join(dir,"data","state.json"));!os.IsNotExist(err){t.Fatal("archive modified data")}
 bad:=createZIP(t,map[string]string{
  prefix+"ZzzMessage.exe":"MZ0000",
  prefix+"web/index.html":"new ui",
  prefix+"web/../../escape":"bad",
 })
 if err:=safeWindowsArchive(bad,t.TempDir());err==nil{t.Fatal("zip traversal not rejected")}
 missing:=createZIP(t,map[string]string{prefix+"ZzzMessage.exe":"MZ0000"})
 if err:=safeWindowsArchive(missing,t.TempDir());err==nil{t.Fatal("missing web accepted")}
}

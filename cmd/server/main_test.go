package main

import (
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
)
func TestStaticSiteAndAPI(t *testing.T) {
 root:=t.TempDir()
 if err:=os.WriteFile(filepath.Join(root,"index.html"),[]byte("<html>Vue App</html>"),0600);err!=nil{t.Fatal(err)}
 if err:=os.Mkdir(filepath.Join(root,"assets"),0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(root,"assets","app.js"),[]byte("console.log('ok')"),0600);err!=nil{t.Fatal(err)}
 api:=http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.WriteHeader(200);_,_=w.Write([]byte("API response"))})
 handler:=withStaticSite(api,root)
 for _,test:=range []struct{path,want string;status int}{
  {"/","Vue App",200},{"/join/friend","Vue App",200},
  {"/assets/app.js","console.log",200},{"/api/health","API response",200},
  {"/assets/missing.js","",404},
 }{
  rr:=httptest.NewRecorder()
  handler.ServeHTTP(rr,httptest.NewRequest("GET",test.path,nil))
  if rr.Code!=test.status{t.Errorf("%s: got %d want %d",test.path,rr.Code,test.status)}
  if test.want!=""&&!strings.Contains(rr.Body.String(),test.want){t.Errorf("%s: missing %q",test.path,test.want)}
 }
}
func TestEnvFileDoesNotOverrideEnvironment(t *testing.T){
 filename:=filepath.Join(t.TempDir(),".env")
 if err:=os.WriteFile(filename,[]byte("INVITE_CODE=fromFile\nSESSION_SECRET=\"some-long-secret\"\n"),0600);err!=nil{t.Fatal(err)}
 t.Setenv("INVITE_CODE","fromProcess")
 if err:=loadEnvFile(filename);err!=nil{t.Fatal(err)}
 if got:=os.Getenv("INVITE_CODE");got!="fromProcess"{t.Fatalf("environment precedence broken: %q",got)}
 if got:=os.Getenv("SESSION_SECRET");got!="some-long-secret"{t.Fatalf("unexpected env: %q",got)}
}

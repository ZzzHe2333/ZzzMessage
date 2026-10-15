package chat

import (
    "bytes"
    "encoding/json"
    "net/http/httptest"
    "path/filepath"
    "testing"
)

func nativeRequest(t *testing.T, s *Server, method, path, bearer string, data any) *httptest.ResponseRecorder {
    t.Helper()
    body,err:=json.Marshal(data)
    if err!=nil {t.Fatal(err)}
    req:=httptest.NewRequest(method,path,bytes.NewReader(body))
    req.Header.Set("Content-Type","application/json")
    if bearer!="" {req.Header.Set("Authorization","Bearer "+bearer)}
    res:=httptest.NewRecorder()
    s.Handler().ServeHTTP(res,req)
    return res
}

func TestNativePushRequiresAuthAndVendorWhitelist(t *testing.T){
    s:=testServer(t)
    alice:=reg(t,s,"alice")
    _=reg(t,s,"bob")
    s.native=&NativePushService{config:NativePushConfig{XiaomiSecret:"test-secret", XiaomiPackage:"io.github.zzzhe2333.zzzmessage"}}

    device:=NativeDevice{Provider:"xiaomi",Token:"device-regid-with-20-chars"}
    if res:=nativeRequest(t,s,"PUT","/api/native-push/device","",device);res.Code!=401{t.Fatalf("unauthenticated device write accepted %d",res.Code)}
    if res:=nativeRequest(t,s,"PUT","/api/native-push/device",alice,NativeDevice{Provider:"unknown",Token:device.Token});res.Code!=400{t.Fatalf("unknown vendor accepted %d",res.Code)}
    if res:=nativeRequest(t,s,"PUT","/api/native-push/device",alice,device);res.Code!=200{t.Fatalf("registration failed: %d %s",res.Code,res.Body.String())}
    if len(s.store.NativeFor("alice"))!=1||len(s.store.NativeFor("bob"))!=0 {t.Fatal("device identity separation failed")}

    loaded,err:=NewStore(filepath.Dir(s.store.path))
    if err!=nil{t.Fatal(err)}
    if len(loaded.NativeFor("alice"))!=1 {t.Fatal("device metadata not persisted")}

    bob:=nativeRequest(t,s,"DELETE","/api/native-push/device",mustLogin(t,s,"bob"),device)
    if bob.Code!=200{t.Fatal(bob.Code)}
    if len(s.store.NativeFor("alice"))!=1{t.Fatal("other user removed Alice device")}
    aliceDel:=nativeRequest(t,s,"DELETE","/api/native-push/device",alice,device)
    if aliceDel.Code!=200 || len(s.store.NativeFor("alice"))!=0 {t.Fatal("unsubscribe failed")}
}
func mustLogin(t *testing.T,s *Server,username string)string{
    t.Helper()
    res:=post(t,s.Handler(),"/api/auth/login",map[string]string{"username":username,"password":"password123"},"")
    if res.Code!=200{t.Fatal(res.Code)}
    var token struct{Token string `json:"token"`}
    if err:=json.Unmarshal(res.Body.Bytes(),&token);err!=nil{t.Fatal(err)}
    return token.Token
}

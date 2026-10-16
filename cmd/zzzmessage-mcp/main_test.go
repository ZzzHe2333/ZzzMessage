package main

import (
 "bytes"
 "context"
 "encoding/json"
 "io"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

func mockZzz(t *testing.T)(*httptest.Server,*int){
 t.Helper()
 logins:=new(int)
 mux:=http.NewServeMux()
 mux.HandleFunc("POST /api/auth/login",func(w http.ResponseWriter,r *http.Request){
  var body map[string]string
  if json.NewDecoder(r.Body).Decode(&body)!=nil||body["username"]!="hermes_bot"||body["password"]!="private-pass"{
   w.WriteHeader(401);return
  }
  *logins=*logins+1
  w.Header().Set("Content-Type","application/json")
  io.WriteString(w,"{\"token\":\"valid-session\"}")
 })
 logged:=func(w http.ResponseWriter,r *http.Request)bool{
  if r.Header.Get("Authorization")!="Bearer valid-session"{w.WriteHeader(401);return false}
  return true
 }
 mux.HandleFunc("GET /api/users",func(w http.ResponseWriter,r *http.Request){
  if !logged(w,r){return}
  io.WriteString(w,"[{\"username\":\"alice\",\"displayName\":\"Alice\"},{\"username\":\"bob\",\"displayName\":\"Bob\"}]")
 })
 mux.HandleFunc("POST /api/direct",func(w http.ResponseWriter,r *http.Request){
  if !logged(w,r){return}
  var body map[string]string
  _=json.NewDecoder(r.Body).Decode(&body)
  if body["username"]!="alice"{w.WriteHeader(400);return}
  w.WriteHeader(201)
  io.WriteString(w,"{\"id\":\"dm_test\"}")
 })
 mux.HandleFunc("POST /api/messages/send",func(w http.ResponseWriter,r *http.Request){
  if !logged(w,r){return}
  var body map[string]string
  _=json.NewDecoder(r.Body).Decode(&body)
  if body["roomId"]!="dm_test"||body["text"]!="自动提醒" {w.WriteHeader(400);return}
  w.WriteHeader(201)
  io.WriteString(w,"{\"id\":\"0123456789abcdef0123456789abcdef\",\"roomId\":\"dm_test\",\"sender\":\"hermes_bot\",\"createdAt\":\"2026-10-16T10:00:00Z\"}")
 })
 return httptest.NewServer(mux),logins
}
func testMCPClient(t *testing.T)(*apiClient,func(),*int){
 t.Helper()
 server,count:=mockZzz(t)
 client:=newClient(config{URL:server.URL,Username:"hermes_bot",Password:"private-pass",AllowedRecipients:map[string]bool{"alice":true}})
 return client,server.Close,count
}
func TestAccountLoginListSendAndRenew(t *testing.T){
 client,done,count:=testMCPClient(t);defer done()
 users,err:=client.listUsers(context.Background())
 if err!=nil||len(users)!=1||users[0].Username!="alice"{t.Fatalf("list: %+v %v",users,err)}
 sent,err:=client.send(context.Background(),"alice","自动提醒")
 if err!=nil||sent.Sender!="hermes_bot"||sent.ID==""{t.Fatalf("send: %+v %v",sent,err)}
 if *count!=1{t.Fatalf("unexpected logins: %d",*count)}
 client.token="stale-session"
 _,err=client.send(context.Background(),"alice","自动提醒")
 if err!=nil||*count!=2{t.Fatalf("token was not refreshed: %v count %d",err,*count)}
 if _,err:=client.send(context.Background(),"bob","blocked");err==nil{t.Fatal("allowlist not enforced")}
 if _,err:=client.send(context.Background(),"hermes_bot","self");err==nil{t.Fatal("self send")}
 if _,err:=client.send(context.Background(),"alice",strings.Repeat("a",4001));err==nil{t.Fatal("length not enforced")}
}
func TestMCPSessionProtocol(t *testing.T){
 client,done,_:=testMCPClient(t);defer done()
 requests:=[]map[string]any{
  {"jsonrpc":"2.0","id":1,"method":"initialize","params":map[string]any{"protocolVersion":"2025-06-18"}},
  {"jsonrpc":"2.0","method":"notifications/initialized"},
  {"jsonrpc":"2.0","id":2,"method":"tools/list"},
  {"jsonrpc":"2.0","id":3,"method":"tools/call","params":map[string]any{"name":"zzzmessage_list_users","arguments":map[string]any{}}},
  {"jsonrpc":"2.0","id":4,"method":"tools/call","params":map[string]any{"name":"zzzmessage_send_private_message","arguments":map[string]any{"recipient":"alice","text":"自动提醒"}}},
 }
 var input bytes.Buffer
 for _,request:=range requests{raw,_:=json.Marshal(request);input.Write(raw);input.WriteByte('\n')}
 var output bytes.Buffer
 if err:=runMCP(context.Background(),&input,&output,client);err!=nil{t.Fatal(err)}
 lines:=strings.Split(strings.TrimSpace(output.String()),"\n")
 if len(lines)!=4{t.Fatalf("notifications must not receive responses: %s",output.String())}
 if strings.Contains(output.String(),"private-pass")||strings.Contains(output.String(),"valid-session"){
  t.Fatal("MCP stdout leaked credentials")
 }
 for i,line:=range lines{
  var response map[string]any
  if err:=json.Unmarshal([]byte(line),&response);err!=nil{t.Fatal(err)}
  if response["error"]!=nil||response["id"]!=float64(i+1)||response["jsonrpc"]!="2.0"{
   t.Fatalf("bad frame: %+v",response)
  }
 }
 if !strings.Contains(lines[1],"zzzmessage_send_private_message")||!strings.Contains(lines[3],"0123456789abcdef0123456789abcdef"){
  t.Fatal("MCP tool declaration or send confirmation missing")
 }
}
func TestMCPConfigRejectsUnencryptedPasswordTransport(t *testing.T){
 for _,candidate:=range []struct{url string;invalid bool}{
  {"http://192.168.1.10:58463",true},
  {"http://127.0.0.1:58463",false},
  {"http://localhost:58463",false},
  {"https://chat.example.org",false},
  {"https://chat.example.org/folder",true},
  {"https://user:pass@chat.example.org",true},
 }{
  t.Setenv("ZZZMESSAGE_URL",candidate.url)
  t.Setenv("ZZZMESSAGE_USERNAME","hermes_bot")
  t.Setenv("ZZZMESSAGE_PASSWORD","sensitive")
  t.Setenv("ZZZMESSAGE_MCP_ALLOWED_RECIPIENTS","alice")
  _,err:=getConfig()
  if (err!=nil)!=candidate.invalid {t.Errorf("URL %s validation: %v",candidate.url,err)}
 }
 if _,_,err:=parseSendArgs(json.RawMessage("{\"recipient\":\"alice\",\"text\":\"hi\",\"password\":\"secret\"}"));err==nil{
  t.Fatal("tool accepted credential injection")
 }
}
func TestMalformedMCPFrameAndNotification(t *testing.T){
 client:=newClient(config{})
 var out bytes.Buffer
 if err:=runMCP(context.Background(),strings.NewReader("{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\"}\n"),&out,client);err!=nil{t.Fatal(err)}
 if out.Len()!=0{t.Fatalf("notification replied: %s",out.String())}
 out.Reset()
 if err:=runMCP(context.Background(),strings.NewReader("bad JSON\n"),&out,client);err!=nil{t.Fatal(err)}
 if !strings.Contains(out.String(),"-32700"){t.Fatal("parse error not reported")}
}

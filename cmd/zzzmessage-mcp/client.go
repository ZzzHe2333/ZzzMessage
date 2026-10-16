package main

import (
 "bufio"
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net"
 "net/http"
 "net/url"
 "os"
 "strings"
 "time"
 "unicode/utf8"
)

const (
 protocolVersion="2025-06-18"
 maxToolInput=4000
 maxMCPFrame=1024*1024
)

type config struct{
 URL string
 Username string
 Password string
 AllowedRecipients map[string]bool
}
func getConfig() (config,error) {
 address:=strings.TrimRight(strings.TrimSpace(os.Getenv("ZZZMESSAGE_URL")),"/")
 login:=strings.ToLower(strings.TrimSpace(os.Getenv("ZZZMESSAGE_USERNAME")))
 password:=os.Getenv("ZZZMESSAGE_PASSWORD")
 if login==""||password=="" {return config{},errors.New("set ZZZMESSAGE_USERNAME and ZZZMESSAGE_PASSWORD in the MCP process environment")}
 u,err:=url.Parse(address)
 if err!=nil || u.Host==""||u.User!=nil||u.RawQuery!=""||u.Fragment!=""||u.Path!=""{
  return config{},errors.New("ZZZMESSAGE_URL must be an HTTPS origin (or http://localhost for local testing)")
 }
 if u.Scheme!="https" {
  host:=u.Hostname()
  ip:=net.ParseIP(host)
  if u.Scheme!="http"||(host!="localhost" && (ip==nil||!ip.IsLoopback())){
   return config{},errors.New("password-based MCP login requires HTTPS; HTTP only allowed on localhost/127.0.0.1")
  }
 }
 allowed:=map[string]bool{}
 for _,name:=range strings.Split(os.Getenv("ZZZMESSAGE_MCP_ALLOWED_RECIPIENTS"),",") {
  name=strings.ToLower(strings.TrimSpace(name))
  if name!=""{
   if !validUsername(name){return config{},errors.New("invalid username in ZZZMESSAGE_MCP_ALLOWED_RECIPIENTS")}
   allowed[name]=true
  }
 }
 if !validUsername(login){return config{},errors.New("ZZZMESSAGE_USERNAME must be a registered account username")}
 return config{URL:address,Username:login,Password:password,AllowedRecipients:allowed},nil
}
func validUsername(u string)bool{
 if len(u)<3||len(u)>24{return false}
 for _,r:=range u {
  if !(r>='a'&&r<='z'||r>='0'&&r<='9'||r=='_'){return false}
 }
 return true
}
type apiClient struct{
 conf config
 http *http.Client
 token string
}
func newClient(c config)*apiClient{
 return &apiClient{conf:c,http:&http.Client{Timeout:15*time.Second,CheckRedirect:func(_ *http.Request,_ []*http.Request)error{
  return http.ErrUseLastResponse
 }}}
}
type apiError struct{Status int; Message string}
func (e apiError) Error()string{return fmt.Sprintf("ZzzMessage HTTP %d: %s",e.Status,e.Message)}
func (c *apiClient) call(ctx context.Context,method,path string,input any,authenticated bool,result any) error {
 var reader io.Reader
 if input!=nil {
  data,err:=json.Marshal(input)
  if err!=nil{return err}
  reader=bytes.NewReader(data)
 }
 request,err:=http.NewRequestWithContext(ctx,method,c.conf.URL+path,reader)
 if err!=nil{return errors.New("unable to create ZzzMessage request")}
 request.Header.Set("Accept","application/json")
 if input!=nil{request.Header.Set("Content-Type","application/json; charset=utf-8")}
 if authenticated{request.Header.Set("Authorization","Bearer "+c.token)}
 reply,err:=c.http.Do(request)
 if err!=nil{return errors.New("cannot connect to ZzzMessage (check URL, HTTPS certificate and network)")}
 defer reply.Body.Close()
 data,err:=io.ReadAll(io.LimitReader(reply.Body,2<<20))
 if err!=nil{return errors.New("cannot read ZzzMessage response")}
 if reply.StatusCode<200||reply.StatusCode>=300 {
  var body struct {Error string `json:"error"`}
  _=json.Unmarshal(data,&body)
  if body.Error==""{body.Error=http.StatusText(reply.StatusCode)}
  if len(body.Error)>240{body.Error=body.Error[:240]}
  return apiError{Status:reply.StatusCode,Message:body.Error}
 }
 if result!=nil && json.Unmarshal(data,result)!=nil {
  return errors.New("ZzzMessage returned invalid JSON")
 }
 return nil
}
func (c *apiClient) login(ctx context.Context)error{
 var response struct{Token string `json:"token"`}
 err:=c.call(ctx,"POST","/api/auth/login",map[string]string{
  "username":c.conf.Username,"password":c.conf.Password,
 },false,&response)
 if err!=nil{return err}
 if response.Token==""{return errors.New("ZzzMessage login did not return a session token")}
 c.token=response.Token
 return nil
}
func (c *apiClient) authed(ctx context.Context,method,path string,body any,result any)error{
 if c.token=="" {
  if err:=c.login(ctx);err!=nil{return err}
 }
 err:=c.call(ctx,method,path,body,true,result)
 var httpErr apiError
 if errors.As(err,&httpErr) && httpErr.Status==401 {
  c.token=""
  if err:=c.login(ctx);err!=nil{return err}
  return c.call(ctx,method,path,body,true,result)
 }
 return err
}
type user struct {
 Username string `json:"username"`
 DisplayName string `json:"displayName"`
}
func (c *apiClient) listUsers(ctx context.Context)([]user,error){
 var users []user
 if err:=c.authed(ctx,"GET","/api/users",nil,&users);err!=nil{return nil,err}
 if len(c.conf.AllowedRecipients)>0{
  filtered:=make([]user,0,len(users))
  for _,u:=range users{if c.conf.AllowedRecipients[u.Username]{filtered=append(filtered,u)}}
  return filtered,nil
 }
 return users,nil
}
type room struct {ID string `json:"id"`}
type sentMessage struct{
 ID string `json:"id"`
 Sender string `json:"sender"`
 RoomID string `json:"roomId"`
 CreatedAt string `json:"createdAt"`
}
func (c *apiClient) send(ctx context.Context,recipient,text string)(sentMessage,error){
 recipient=strings.ToLower(strings.TrimSpace(recipient))
 if !validUsername(recipient)||recipient==c.conf.Username {
  return sentMessage{},errors.New("recipient must be another registered username (3–24 lowercase characters, numbers or underscore)")
 }
 if len(c.conf.AllowedRecipients)>0&&!c.conf.AllowedRecipients[recipient] {
  return sentMessage{},errors.New("recipient not allowed by ZZZMESSAGE_MCP_ALLOWED_RECIPIENTS")
 }
 if strings.TrimSpace(text)==""||!utf8.ValidString(text)||utf8.RuneCountInString(text)>maxToolInput {
  return sentMessage{},errors.New("message text must be 1–4000 Unicode characters")
 }
 var direct room
 if err:=c.authed(ctx,"POST","/api/direct",map[string]string{"username":recipient},&direct);err!=nil{return sentMessage{},err}
 if direct.ID==""{return sentMessage{},errors.New("private conversation ID missing")}
 var result sentMessage
 if err:=c.authed(ctx,"POST","/api/messages/send",map[string]string{
  "roomId":direct.ID,"text":text,
 },&result);err!=nil{return sentMessage{},err}
 if result.ID==""||result.RoomID!=direct.ID||result.Sender!=c.conf.Username{
  return sentMessage{},errors.New("server response did not confirm the expected sending account and conversation")
 }
 return result,nil
}

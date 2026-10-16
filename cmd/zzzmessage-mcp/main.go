package main

import (
 "bufio"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "os"
 "strings"
 "time"
)

type mcpMessage struct {
 JSONRPC string `json:"jsonrpc"`
 ID json.RawMessage `json:"id,omitempty"`
 Method string `json:"method"`
 Params json.RawMessage `json:"params,omitempty"`
}
type mcpResponse struct {
 JSONRPC string `json:"jsonrpc"`
 ID json.RawMessage `json:"id"`
 Result any `json:"result,omitempty"`
 Error any `json:"error,omitempty"`
}
type mcpError struct{
 Code int `json:"code"`
 Message string `json:"message"`
}
func replyError(id json.RawMessage,code int,message string)mcpResponse{
 if len(id)==0{id=json.RawMessage("null")}
 return mcpResponse{JSONRPC:"2.0",ID:id,Error:mcpError{Code:code,Message:message}}
}
func asTextResult(value any,problem error)any{
 if problem!=nil {
  return map[string]any{"isError":true,"content":[]map[string]string{{"type":"text","text":problem.Error()}}}
 }
 encoded,err:=json.Marshal(value)
 if err!=nil {encoded=[]byte("{}")}
 return map[string]any{"isError":false,"content":[]map[string]string{{"type":"text","text":string(encoded)}}}
}
func toolsList()any{
 return map[string]any{"tools":[]any{
  map[string]any{
   "name":"zzzmessage_list_users",
   "title":"List ZzzMessage contacts",
   "description":"List registered recipient usernames and display names visible to the configured ZzzMessage account. Does not read messages.",
   "inputSchema":map[string]any{"type":"object","properties":map[string]any{},"additionalProperties":false},
   "annotations":map[string]any{"readOnlyHint":true,"destructiveHint":false,"openWorldHint":false},
  },
  map[string]any{
   "name":"zzzmessage_send_private_message",
   "title":"Send a ZzzMessage private message",
   "description":"Send a private text message as the ZzzMessage account configured in the MCP process. This action communicates with another user. Require the user's approval before sending. Sender identity cannot be changed by the tool caller.",
   "inputSchema":map[string]any{
    "type":"object","properties":map[string]any{
     "recipient":map[string]any{"type":"string","description":"Registered ZzzMessage username, not display name"},
     "text":map[string]any{"type":"string","description":"The message body (1–4000 characters)"},
    },
    "required":[]string{"recipient","text"},"additionalProperties":false,
   },
   "annotations":map[string]any{"readOnlyHint":false,"destructiveHint":false,"openWorldHint":true,"idempotentHint":false},
  },
 }}
}
func decodeCallParams(raw json.RawMessage)(string,json.RawMessage,error){
 var params struct {
  Name string `json:"name"`
  Arguments json.RawMessage `json:"arguments"`
 }
 if err:=json.Unmarshal(raw,&params);err!=nil{return "",nil,errors.New("invalid MCP tool parameters")}
 if params.Name==""{return "",nil,errors.New("tool name required")}
 return params.Name,params.Arguments,nil
}
func parseSendArgs(raw json.RawMessage)(string,string,error){
 var args struct{
  Recipient string `json:"recipient"`
  Text string `json:"text"`
 }
 if len(raw)==0{return "","",errors.New("recipient and text required")}
 d:=json.NewDecoder(strings.NewReader(string(raw)))
 d.DisallowUnknownFields()
 if err:=d.Decode(&args);err!=nil{return "","",errors.New("expected recipient and text only")}
 if args.Recipient==""||args.Text==""{return "","",errors.New("recipient and text are required")}
 return args.Recipient,args.Text,nil
}
func execute(ctx context.Context,req mcpMessage,client *apiClient) *mcpResponse{
 if len(req.ID)==0{
  // JSON-RPC notifications never receive a response.
  return nil
 }
 response:=&mcpResponse{JSONRPC:"2.0",ID:req.ID}
 switch req.Method {
 case "initialize":
  response.Result=map[string]any{
   "protocolVersion":protocolVersion,
   "capabilities":map[string]any{"tools":map[string]any{"listChanged":false}},
   "serverInfo":map[string]string{"name":"zzzmessage-mcp","version":"0.1.0"},
   "instructions":"The AI may list recipients and send private messages only as the configured ZzzMessage account. Ask for user approval before sending. Username/password and session tokens never appear as tool arguments.",
  }
 case "ping":
  response.Result=map[string]any{}
 case "tools/list":
  response.Result=toolsList()
 case "tools/call":
  name,args,err:=decodeCallParams(req.Params)
  if err!=nil{response.Result=asTextResult(nil,err);return response}
  switch name {
  case "zzzmessage_list_users":
   if len(args)>0&&string(args)!="{}"&&string(args)!="null" {
    response.Result=asTextResult(nil,errors.New("list_users accepts no arguments"))
    return response
   }
   timeout,cancel:=context.WithTimeout(ctx,20*time.Second)
   defer cancel()
   users,err:=client.listUsers(timeout)
   response.Result=asTextResult(map[string]any{"account":client.conf.Username,"users":users},err)
  case "zzzmessage_send_private_message":
   recipient,text,err:=parseSendArgs(args)
   if err!=nil{response.Result=asTextResult(nil,err);return response}
   timeout,cancel:=context.WithTimeout(ctx,40*time.Second)
   defer cancel()
   message,err:=client.send(timeout,recipient,text)
   response.Result=asTextResult(map[string]any{
    "ok":err==nil,"from":client.conf.Username,"to":recipient,
    "messageId":message.ID,"roomId":message.RoomID,"createdAt":message.CreatedAt,
   },err)
  default:
   response.Result=asTextResult(nil,fmt.Errorf("unknown ZzzMessage MCP tool: %s",name))
  }
 default:
  problem:=replyError(req.ID,-32601,"method not found")
  return &problem
 }
 return response
}
func runMCP(ctx context.Context,reader io.Reader,writer io.Writer,client *apiClient)error{
 scanner:=bufio.NewScanner(reader)
 scanner.Buffer(make([]byte,4096),maxMCPFrame)
 buffered:=bufio.NewWriter(writer)
 for scanner.Scan() {
  raw:=scanner.Bytes()
  if len(strings.TrimSpace(string(raw)))==0{continue}
  var request mcpMessage
  if err:=json.Unmarshal(raw,&request);err!=nil {
   result:=replyError(nil,-32700,"parse error")
   data,_:=json.Marshal(result)
   if _,err:=buffered.Write(append(data,'\n'));err!=nil{return err}
   if err:=buffered.Flush();err!=nil{return err}
   continue
  }
  if request.JSONRPC!="2.0"||request.Method==""{
   errReply:=replyError(request.ID,-32600,"invalid JSON-RPC request")
   data,_:=json.Marshal(errReply)
   if _,err:=buffered.Write(append(data,'\n'));err!=nil{return err}
   if err:=buffered.Flush();err!=nil{return err}
   continue
  }
  response:=execute(ctx,request,client)
  if response==nil{continue}
  data,err:=json.Marshal(response)
  if err!=nil{return err}
  if _,err:=buffered.Write(append(data,'\n'));err!=nil{return err}
  if err:=buffered.Flush();err!=nil{return err}
 }
 return scanner.Err()
}
func main(){
 c,err:=getConfig()
 if err!=nil {
  fmt.Fprintln(os.Stderr,"ZzzMessage MCP configuration error:",err)
  os.Exit(2)
 }
 // The process writes ONLY JSON-RPC frames on stdout. Diagnostics go to stderr.
 if err:=runMCP(context.Background(),os.Stdin,os.Stdout,newClient(c));err!=nil{
  fmt.Fprintln(os.Stderr,"ZzzMessage MCP transport error:",err)
  os.Exit(1)
 }
}

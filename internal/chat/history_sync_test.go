package chat

import (
  "encoding/json"
  "net/http/httptest"
  "strings"
  "testing"
  "time"

  "github.com/gorilla/websocket"
)

func TestHistorySyncIsAccountIsolated(t *testing.T){
    s:=testServer(t)
    alice:=reg(t,s,"alice")
    bob:=reg(t,s,"bob")
    host:=httptest.NewServer(s.Handler())
    defer host.Close()
    dial:=func(token,device string)*websocket.Conn{
        t.Helper()
        ws,_,err:=websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(host.URL,"http")+"/api/ws",nil)
        if err!=nil{t.Fatal(err)}
        ws.SetReadDeadline(time.Now().Add(3*time.Second))
        if err:=ws.WriteJSON(map[string]string{"type":"auth","token":token,"deviceId":device});err!=nil{t.Fatal(err)}
        var ready struct{Type string `json:"type"`}
        if err:=ws.ReadJSON(&ready);err!=nil||ready.Type!="ready"{t.Fatal("ready",err,ready)}
        return ws
    }
    alice1:=dial(alice,"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
    defer alice1.Close()
    alice2:=dial(alice,"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
    defer alice2.Close()
    bob1:=dial(bob,"cccccccccccccccccccccccccccccccc")
    defer bob1.Close()
    if err:=alice2.WriteJSON(map[string]string{"type":"sync_request"});err!=nil{t.Fatal(err)}
    var ask struct {
        Type string `json:"type"`
        RequestID string `json:"requestId"`
        TargetDevice string `json:"targetDevice"`
    }
    if err:=alice1.ReadJSON(&ask);err!=nil||ask.Type!="sync_request"||ask.TargetDevice!="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"{t.Fatal("same-account request",ask,err)}
    // Bob cannot use Alice's pending sync request or its device identifiers.
    bobConn:=&Client{username:"bob",deviceID:"cccccccccccccccccccccccccccccccc"}
    if s.permittedHistoryRequest(bobConn,ask.RequestID,ask.TargetDevice,false){t.Fatal("cross-account history access")}
    if !s.permittedHistoryRequest(&Client{username:"alice",deviceID:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},ask.RequestID,ask.TargetDevice,false){
        t.Fatal("authenticated source denied")
    }
    jsonBytes,_:=json.Marshal(map[string]any{"type":"sync_chunk","requestId":ask.RequestID,"targetDevice":ask.TargetDevice,"seq":0,"messages":[]ChatMessage{{
        Type:"message",ID:"11111111111111111111111111111111",RoomID:"lobby",Sender:"alice",Text:"hello",
        CreatedAt:time.Now().UTC().Format(time.RFC3339Nano),
    }}})
    if err:=alice1.WriteMessage(websocket.TextMessage,jsonBytes);err!=nil{t.Fatal(err)}
    var status map[string]any
    if err:=alice2.ReadJSON(&status);err!=nil{t.Fatal(err)} // sync_status from server
    if status["type"]!="sync_status"{t.Fatal("expected status",status)}
    if err:=alice2.ReadJSON(&status);err!=nil||status["type"]!="sync_chunk"{t.Fatal("sync chunk",err,status)}
    if status["sourceDevice"]!="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"{t.Fatal("wrong sender",status)}
    if err:=alice2.WriteJSON(map[string]any{"type":"sync_chunk_ack","requestId":ask.RequestID,
       "deviceId":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","seq":0});err!=nil{t.Fatal(err)}
    if err:=alice1.ReadJSON(&status);err!=nil||status["type"]!="sync_chunk_ack"{t.Fatal("ack forwarding",err,status)}
}

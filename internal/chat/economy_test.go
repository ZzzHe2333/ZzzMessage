package chat

import (
  "encoding/json"
  "net/http"
  "net/http/httptest"
  "path/filepath"
  "testing"
  "time"
)

func TestVirtualCoinsDailyPersistence(t *testing.T) {
    s:=testServer(t)
    _=reg(t,s,"alice")
    now:=time.Date(2026,10,15,12,30,0,0,time.UTC)
    before,err:=s.store.GetWallet("alice")
    if err!=nil || before.Gold!=0||before.Silver!=0||before.Copper!=0 {t.Fatal("initial wallet",before,err)}
    earned,err:=s.store.ClaimDaily("alice",now)
    if err!=nil || earned.Gold!=1||earned.Silver!=5||earned.Copper!=150 {t.Fatal("daily reward",earned,err)}
    _,err=s.store.ClaimDaily("alice",now.Add(3*time.Hour))
    if err!=ErrAlreadyClaimed{t.Fatal("duplicate daily claim accepted",err)}
    reread,err:=NewStore(filepath.Dir(s.store.path))
    if err!=nil{t.Fatal(err)}
    saved,err:=reread.GetWallet("alice")
    if err!=nil || saved!=earned{t.Fatal("daily reward lost after restart",saved,err)}
    tomorrow,err:=reread.ClaimDaily("alice",now.Add(24*time.Hour))
    if err!=nil || tomorrow.Gold!=2||tomorrow.Silver!=10||tomorrow.Copper!=300{t.Fatal("tomorrow reward",tomorrow,err)}
}

func TestGiftAtomicAndIdempotent(t *testing.T){
    s:=testServer(t)
    _=reg(t,s,"alice")
    _=reg(t,s,"bob")
    now:=time.Now().UTC()
    _,err:=s.store.ClaimDaily("alice",now)
    if err!=nil{t.Fatal(err)}
    id:="0123456789abcdef0123456789abcdef"
    bought,wallet,err:=s.store.BuyAndSendGift("alice","bob","flower",id,now)
    if err!=nil||bought.Price!=50||wallet.Copper!=100{t.Fatal("purchase failed",bought,wallet,err)}
    same,again,err:=s.store.BuyAndSendGift("alice","bob","flower",id,now.Add(time.Minute))
    if err!=nil||same.RequestID!=id||again.Copper!=100{t.Fatal("retry double charged",same,again,err)}
    _,_,err=s.store.BuyAndSendGift("alice","bob","coffee",id,now)
    if err!=ErrGiftRequestConflict{t.Fatal("conflicting request reused id",err)}
    _,_,err=s.store.BuyAndSendGift("alice","alice","flower","22222222222222222222222222222222",now)
    if err!=ErrCannotGiftSelf{t.Fatal("self gifting accepted",err)}
    _,_,err=s.store.BuyAndSendGift("bob","alice","flower","33333333333333333333333333333333",now)
    if err!=ErrInsufficientFunds{t.Fatal("insufficient coins accepted",err)}
    _,_,err=s.store.BuyAndSendGift("alice","nobody","flower","44444444444444444444444444444444",now)
    if err!=ErrNotFound{t.Fatal("nonexistent gift recipient accepted",err)}
    if got:=s.store.GiftsFor("bob");len(got)!=1 || got[0].Sender!="alice" {t.Fatal("recipient history",got)}
    reloaded,err:=NewStore(filepath.Dir(s.store.path))
    if err!=nil{t.Fatal(err)}
    if len(reloaded.GiftsFor("alice"))!=1{t.Fatal("gift ledger not persisted")}
    balance,_:=reloaded.GetWallet("alice")
    if balance.Copper!=100{t.Fatal("reloaded balance",balance)}
}

func TestEconomyHTTPRequiresAccount(t *testing.T){
    s:=testServer(t)
    alice:=reg(t,s,"alice")
    _=reg(t,s,"bob")
    handler:=s.Handler()
    for _,route:=range []string{"/api/money","/api/gifts","/api/gifts/history"}{
        req:=httptest.NewRequest(http.MethodGet,route,nil)
        out:=httptest.NewRecorder()
        handler.ServeHTTP(out,req)
        if out.Code!=http.StatusUnauthorized{t.Fatalf("anonymous %s accepted",route)}
    }
    earned:=post(t,handler,"/api/money/daily",map[string]any{},alice)
    if earned.Code!=200{t.Fatal("daily HTTP",earned.Code,earned.Body.String())}
    giftsReq:=httptest.NewRequest(http.MethodGet,"/api/gifts",nil)
    giftsReq.Header.Set("Authorization","Bearer "+alice)
    record:=httptest.NewRecorder()
    handler.ServeHTTP(record,giftsReq)
    var gifts []GiftItem
    if err:=json.Unmarshal(record.Body.Bytes(),&gifts);err!=nil||len(gifts)!=6{t.Fatal("gift catalog HTTP",err,gifts)}
    sent:=post(t,handler,"/api/gifts/send",map[string]string{
        "recipient":"bob","itemId":"flower","requestId":"abcdef0123456789abcdef0123456789",
    },alice)
    if sent.Code!=200{t.Fatal("gift HTTP",sent.Code,sent.Body.String())}
}

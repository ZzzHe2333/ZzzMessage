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
    before,err:=s.store.GetWallet("alice")
    if err!=nil{t.Fatal(err)}
    // Successful registration also authenticates the user, awarding one coin.
    if before.Gold+before.Silver+before.Copper!=1 || before.LastReward==""{
        t.Fatal("first registered session should award exactly one coin",before)
    }
    future:=time.Date(2035,10,15,12,30,0,0,time.UTC)
    got,err:=s.store.claimDailyWithRoll("alice",future,func()(int,error){return 0,nil})
    if err!=nil||got.Gold!=before.Gold+1||got.Silver!=before.Silver||got.Copper!=before.Copper||
        got.LastReward!="gold" {t.Fatal("gold boundary failed",got,err)}
    // No draw is allowed on a second login on the same UTC date.
    _,err=s.store.claimDailyWithRoll("alice",future.Add(time.Hour),func()(int,error){
        t.Fatal("duplicate daily reward consumed randomness")
        return 0,nil
    })
    if err!=ErrAlreadyClaimed{t.Fatal("duplicate reward was allowed",err)}
    reloaded,err:=NewStore(filepath.Dir(s.store.path))
    if err!=nil{t.Fatal(err)}
    saved,err:=reloaded.GetWallet("alice")
    if err!=nil||saved!=got{t.Fatal("daily reward did not persist",saved,err)}
    next,err:=reloaded.claimDailyWithRoll("alice",future.Add(24*time.Hour),func()(int,error){return 1,nil})
    if err!=nil||next.Silver!=got.Silver+1||next.LastReward!="silver"{t.Fatal("silver boundary failed",next,err)}
    third,err:=reloaded.claimDailyWithRoll("alice",future.Add(48*time.Hour),func()(int,error){return 10,nil})
    if err!=nil||third.Copper!=next.Copper+1||third.LastReward!="copper"{t.Fatal("copper boundary failed",third,err)}
}
func TestDailyRewardWeights(t *testing.T){
    counts:=map[string]int{}
    for roll:=0;roll<100;roll++{counts[dailyCurrency(roll)]++}
    if counts["gold"]!=1||counts["silver"]!=9||counts["copper"]!=90{
        t.Fatalf("wrong daily lottery weights: %+v",counts)
    }
    if dailyCurrency(-1)!=""||dailyCurrency(100)!=""{t.Fatal("invalid draw was accepted")}
}
func TestDailyLoginOnceAcrossDevicesAndSessions(t *testing.T){
    s:=testServer(t)
    alice:=reg(t,s,"alice")
    first,err:=s.store.GetWallet("alice")
    if err!=nil||first.Gold+first.Silver+first.Copper!=1{t.Fatal("register award",first,err)}
    login:=post(t,s.Handler(),"/api/auth/login",map[string]string{"username":"alice","password":"password123"},"")
    if login.Code!=200{t.Fatal("login failed",login.Code,login.Body.String())}
    var loginResponse struct{ DailyReward *struct{Currency string} `json:"dailyReward"` }
    if err:=json.Unmarshal(login.Body.Bytes(),&loginResponse);err!=nil{t.Fatal(err)}
    if loginResponse.DailyReward!=nil{t.Fatal("duplicate login awarded another coin")}
    again,err:=s.store.GetWallet("alice")
    if err!=nil||again!=first{t.Fatal("login credited wallet a second time",again,err)}
    // Persistent PWA session check-in is also idempotent.
    for i:=0;i<3;i++{
        reply:=post(t,s.Handler(),"/api/money/daily",map[string]any{},alice)
        if reply.Code!=200{t.Fatal("check-in status",reply.Code,reply.Body.String())}
        var state struct{
            Claimed bool `json:"claimed"`
            Wallet Wallet `json:"wallet"`
        }
        if err:=json.Unmarshal(reply.Body.Bytes(),&state);err!=nil{t.Fatal(err)}
        if state.Claimed||state.Wallet!=first{t.Fatal("idempotent check-in duplicated reward",state)}
    }
    // Simulate the next UTC day and authenticate from a second device.
    s.store.mu.Lock()
    edited:=s.store.state.Wallets["alice"]
    edited.LastDaily=time.Now().UTC().Add(-24*time.Hour).Format("2006-01-02")
    s.store.state.Wallets["alice"]=edited
    s.store.mu.Unlock()
    nextLogin:=post(t,s.Handler(),"/api/auth/login",map[string]string{"username":"alice","password":"password123"},"")
    if nextLogin.Code!=200{t.Fatal(nextLogin.Code,nextLogin.Body.String())}
    var next struct{DailyReward *struct{Currency string; Amount int} `json:"dailyReward"`}
    if err:=json.Unmarshal(nextLogin.Body.Bytes(),&next);err!=nil{t.Fatal(err)}
    if next.DailyReward==nil||next.DailyReward.Amount!=1{t.Fatal("next-day login did not award one coin",next)}
    final,_:=s.store.GetWallet("alice")
    if final.Gold+final.Silver+final.Copper!=2{t.Fatal("wrong accumulated coin count",final)}
}
func TestGiftAtomicAndIdempotent(t *testing.T){
    s:=testServer(t)
    _=reg(t,s,"alice")
    _=reg(t,s,"bob")
    now:=time.Now().UTC()
    // Deterministic seed for this gift test; sign-in rewards are random.
    s.store.mu.Lock()
    seeded:=s.store.state.Wallets["alice"]
    seeded.Copper=150
    s.store.state.Wallets["alice"]=seeded
    s.store.mu.Unlock()
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

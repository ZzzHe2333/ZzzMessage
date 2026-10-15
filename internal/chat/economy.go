package chat

import (
  "errors"
  "net/http"
  "sort"
  "time"
)

// Entirely virtual currency. No payment, fiat conversion, withdrawals,
// gambling, or user-to-user coin transfer is supported.
type Wallet struct {
    Gold int64 `json:"gold"`
    Silver int64 `json:"silver"`
    Copper int64 `json:"copper"`
    LastDaily string `json:"lastDaily,omitempty"`
}
type GiftItem struct{
    ID string `json:"id"`
    Name string `json:"name"`
    Icon string `json:"icon"`
    Currency string `json:"currency"`
    Price int64 `json:"price"`
    Description string `json:"description"`
}
type GiftPurchase struct {
    RequestID string `json:"requestId"`
    Sender string `json:"sender"`
    Recipient string `json:"recipient"`
    ItemID string `json:"itemId"`
    ItemName string `json:"itemName"`
    Currency string `json:"currency"`
    Price int64 `json:"price"`
    CreatedAt string `json:"createdAt"`
}
var giftItems = []GiftItem{
    {"flower","鲜花","🌷","copper",50,"送上一束鲜花"},
    {"coffee","热咖啡","☕","copper",120,"来杯热咖啡"},
    {"cake","生日蛋糕","🎂","silver",3,"庆祝重要的日子"},
    {"teddy","玩偶熊","🧸","silver",12,"毛绒绒的心意"},
    {"star","流星","🌠","gold",2,"点亮一颗星"},
    {"castle","梦幻城堡","🏰","gold",8,"专属梦幻城堡"},
}
var ErrInsufficientFunds=errors.New("虚拟币余额不足")
var ErrAlreadyClaimed=errors.New("今天已领取奖励")
var ErrCannotGiftSelf=errors.New("不能向自己赠送礼物")
var ErrGiftRequestConflict=errors.New("礼物请求 ID 已被其他交易使用")

func (s *Store) GetWallet(username string)(Wallet,error){
    s.mu.RLock();defer s.mu.RUnlock()
    if _,ok:=s.state.Users[username];!ok{return Wallet{},ErrNotFound}
    return s.state.Wallets[username],nil
}
func (s *Store) ClaimDaily(username string,now time.Time)(Wallet,error){
    date:=now.UTC().Format("2006-01-02")
    s.mu.Lock();defer s.mu.Unlock()
    if _,ok:=s.state.Users[username];!ok{return Wallet{},ErrNotFound}
    old:=s.state.Wallets[username]
    if old.LastDaily==date{return old,ErrAlreadyClaimed}
    earned:=old
    earned.Gold+=1
    earned.Silver+=5
    earned.Copper+=150
    earned.LastDaily=date
    if s.state.Wallets==nil{s.state.Wallets=map[string]Wallet{}}
    s.state.Wallets[username]=earned
    if err:=s.saveLocked();err!=nil{s.state.Wallets[username]=old;return Wallet{},err}
    return earned,nil
}
func (s *Store) BuyAndSendGift(sender,recipient,itemID,requestID string,now time.Time)(GiftPurchase,Wallet,error){
    if len(requestID)!=32{return GiftPurchase{},Wallet{},ErrInvalid}
    for _,c:=range requestID{if !((c>='0'&&c<='9')||(c>='a'&&c<='f')){return GiftPurchase{},Wallet{},ErrInvalid}}
    if sender==recipient{return GiftPurchase{},Wallet{},ErrCannotGiftSelf}
    var item GiftItem
    found:=false
    for _,v:=range giftItems{if v.ID==itemID{item=v;found=true;break}}
    if !found{return GiftPurchase{},Wallet{},ErrInvalid}
    s.mu.Lock();defer s.mu.Unlock()
    if _,ok:=s.state.Users[sender];!ok{return GiftPurchase{},Wallet{},ErrNotFound}
    if _,ok:=s.state.Users[recipient];!ok{return GiftPurchase{},Wallet{},ErrNotFound}
    if existing,ok:=s.state.GiftLedger[requestID];ok{
        if existing.Sender!=sender||existing.Recipient!=recipient||existing.ItemID!=itemID{
            return GiftPurchase{},Wallet{},ErrGiftRequestConflict
        }
        return existing,s.state.Wallets[sender],nil
    }
    previous:=s.state.Wallets[sender]
    updated:=previous
    switch item.Currency{
    case "copper":
        if updated.Copper<item.Price{return GiftPurchase{},Wallet{},ErrInsufficientFunds}
        updated.Copper-=item.Price
    case "silver":
        if updated.Silver<item.Price{return GiftPurchase{},Wallet{},ErrInsufficientFunds}
        updated.Silver-=item.Price
    case "gold":
        if updated.Gold<item.Price{return GiftPurchase{},Wallet{},ErrInsufficientFunds}
        updated.Gold-=item.Price
    default:return GiftPurchase{},Wallet{},ErrInvalid
    }
    record:=GiftPurchase{RequestID:requestID,Sender:sender,Recipient:recipient,ItemID:item.ID,
        ItemName:item.Name,Currency:item.Currency,Price:item.Price,CreatedAt:now.UTC().Format(time.RFC3339Nano)}
    if s.state.GiftLedger==nil{s.state.GiftLedger=map[string]GiftPurchase{}}
    if s.state.Wallets==nil{s.state.Wallets=map[string]Wallet{}}
    s.state.Wallets[sender]=updated
    s.state.GiftLedger[requestID]=record
    if err:=s.saveLocked();err!=nil{
        s.state.Wallets[sender]=previous
        delete(s.state.GiftLedger,requestID)
        return GiftPurchase{},Wallet{},err
    }
    return record,updated,nil
}
func (s *Store) GiftsFor(username string)[]GiftPurchase{
    s.mu.RLock();defer s.mu.RUnlock()
    gifts:=make([]GiftPurchase,0)
    for _,v:=range s.state.GiftLedger{
        if v.Sender==username||v.Recipient==username{gifts=append(gifts,v)}
    }
    sort.Slice(gifts,func(i,j int)bool{return gifts[i].CreatedAt>gifts[j].CreatedAt})
    if len(gifts)>200{gifts=gifts[:200]}
    return gifts
}
func (s *Server) moneyBalance(w http.ResponseWriter,r *http.Request,user string){
    wallet,err:=s.store.GetWallet(user)
    if err!=nil{apiError(w,404,"账户不存在");return}
    writeJSON(w,200,wallet)
}
func (s *Server) moneyDaily(w http.ResponseWriter,r *http.Request,user string){
    wallet,err:=s.store.ClaimDaily(user,time.Now())
    if errors.Is(err,ErrAlreadyClaimed){apiError(w,409,"今日奖励已经领取，UTC 次日可重新领取");return}
    if err!=nil{apiError(w,500,"领取失败");return}
    writeJSON(w,200,wallet)
}
func (s *Server) giftCatalog(w http.ResponseWriter,r *http.Request,user string){
    writeJSON(w,200,giftItems)
}
func (s *Server) giftHistory(w http.ResponseWriter,r *http.Request,user string){
    writeJSON(w,200,s.store.GiftsFor(user))
}
func (s *Server) giftSend(w http.ResponseWriter,r *http.Request,user string){
    var request struct{
        Recipient string `json:"recipient"`
        ItemID string `json:"itemId"`
        RequestID string `json:"requestId"`
    }
    if !readJSON(w,r,&request){return}
    purchase,wallet,err:=s.store.BuyAndSendGift(user,request.Recipient,request.ItemID,request.RequestID,time.Now())
    switch{
    case errors.Is(err,ErrInsufficientFunds):apiError(w,409,err.Error());return
    case errors.Is(err,ErrCannotGiftSelf),errors.Is(err,ErrInvalid):apiError(w,400,err.Error());return
    case errors.Is(err,ErrNotFound):apiError(w,404,"收件人不存在");return
    case errors.Is(err,ErrGiftRequestConflict):apiError(w,409,err.Error());return
    case err!=nil:apiError(w,500,"购买礼物失败");return
    }
    s.hub.sendToUser(request.Recipient,map[string]any{"type":"gift_received","from":user,"itemName":purchase.ItemName})
    writeJSON(w,200,map[string]any{"purchase":purchase,"wallet":wallet})
}

package chat

import (
  "crypto/rand"
  "crypto/sha256"
  "crypto/subtle"
  "encoding/hex"
  "errors"
  "net/http"
  "sort"
  "strings"
  "time"
  "unicode/utf8"
)

const maxAPIKeysPerAccount = 5

// APIKeyRecord stores only a digest. The raw key is returned once on creation.
type APIKeyRecord struct {
  ID string `json:"id"`
  Owner string `json:"owner"`
  Label string `json:"label"`
  Recipients []string `json:"recipients"`
  KeyHash string `json:"hash"`
  Suffix string `json:"suffix"`
  CreatedAt string `json:"createdAt"`
  ExpiresAt string `json:"expiresAt"`
}
type PublicAPIKey struct {
  ID string `json:"id"`
  Label string `json:"label"`
  Recipients []string `json:"recipients"`
  Suffix string `json:"suffix"`
  CreatedAt string `json:"createdAt"`
  ExpiresAt string `json:"expiresAt"`
}
func (rec APIKeyRecord) Public() PublicAPIKey {
  return PublicAPIKey{ID:rec.ID,Label:rec.Label,Recipients:append([]string{},rec.Recipients...),Suffix:rec.Suffix,CreatedAt:rec.CreatedAt,ExpiresAt:rec.ExpiresAt}
}
func (s *Store) ListAPIKeys(owner string) []PublicAPIKey {
  s.mu.RLock(); defer s.mu.RUnlock()
  result:=make([]PublicAPIKey,0)
  for _,key:=range s.state.APIKeys {if key.Owner==owner {result=append(result,key.Public())}}
  sort.Slice(result,func(i,j int)bool{return result[i].CreatedAt>result[j].CreatedAt})
  return result
}
func (s *Store) HasAPIKeys()bool{
  s.mu.RLock();defer s.mu.RUnlock()
  return len(s.state.APIKeys)>0
}
func (s *Store) NewAPIKey(owner,label string,recipients []string,validDays int)(PublicAPIKey,string,error){
  label=strings.TrimSpace(label)
  if label==""||utf8.RuneCountInString(label)>48 {return PublicAPIKey{},"",ErrInvalid}
  if validDays!=30&&validDays!=90&&validDays!=365{return PublicAPIKey{},"",ErrInvalid}
  if len(recipients)==0||len(recipients)>30{return PublicAPIKey{},"",ErrInvalid}
  uniq:=make(map[string]bool,len(recipients))
  for _,user:=range recipients {
    // Do not silently convert the recipient identity or permit duplicates.
    if !validUsername(user)||user==owner||uniq[user] {return PublicAPIKey{},"",ErrInvalid}
    uniq[user]=true
  }
  bytes:=make([]byte,32)
  if _,err:=rand.Read(bytes);err!=nil{return PublicAPIKey{},"",err}
  raw:="zzm_"+hex.EncodeToString(bytes)
  sum:=sha256.Sum256([]byte(raw))
  id,err:=randomID();if err!=nil{return PublicAPIKey{},"",err}
  now:=time.Now().UTC()
  rec:=APIKeyRecord{ID:id,Owner:owner,Label:label,Recipients:append([]string{},recipients...),
    KeyHash:hex.EncodeToString(sum[:]),Suffix:raw[len(raw)-4:],
    CreatedAt:now.Format(time.RFC3339),ExpiresAt:now.AddDate(0,0,validDays).Format(time.RFC3339)}
  s.mu.Lock();defer s.mu.Unlock()
  if _,ok:=s.state.Users[owner];!ok{return PublicAPIKey{},"",ErrNotFound}
  for _,user:=range rec.Recipients {if _,ok:=s.state.Users[user];!ok{return PublicAPIKey{},"",ErrNotFound}}
  total:=0
  for _,key:=range s.state.APIKeys {if key.Owner==owner {total++}}
  if total>=maxAPIKeysPerAccount {return PublicAPIKey{},"",ErrExists}
  if s.state.APIKeys==nil{s.state.APIKeys=map[string]APIKeyRecord{}}
  s.state.APIKeys[id]=rec
  if err:=s.saveLocked();err!=nil{delete(s.state.APIKeys,id);return PublicAPIKey{},"",err}
  return rec.Public(),raw,nil
}
func (s *Store) RevokeAPIKey(owner,id string) error {
  s.mu.Lock();defer s.mu.Unlock()
  rec,ok:=s.state.APIKeys[id]
  if !ok||rec.Owner!=owner{return ErrNotFound}
  delete(s.state.APIKeys,id)
  if err:=s.saveLocked();err!=nil{s.state.APIKeys[id]=rec;return err}
  return nil
}
func (s *Store) ResolveAPIKey(raw string)(APIKeyRecord,bool){
  if !strings.HasPrefix(raw,"zzm_")||len(raw)!=68 {return APIKeyRecord{},false}
  hash:=sha256.Sum256([]byte(raw))
  encoded:=hex.EncodeToString(hash[:])
  s.mu.RLock();defer s.mu.RUnlock()
  for _,key:=range s.state.APIKeys {
    if subtle.ConstantTimeCompare([]byte(encoded),[]byte(key.KeyHash))!=1{continue}
    expires,err:=time.Parse(time.RFC3339,key.ExpiresAt)
    if err!=nil||!time.Now().Before(expires){return APIKeyRecord{},false}
    return key,true
  }
  return APIKeyRecord{},false
}
func (s *Server) listUserAPIKeys(w http.ResponseWriter,_ *http.Request,owner string){
  writeJSON(w,200,map[string]any{"keys":s.store.ListAPIKeys(owner),"maxKeys":maxAPIKeysPerAccount})
}
func (s *Server) createUserAPIKey(w http.ResponseWriter,r *http.Request,owner string){
  var data struct{Label string `json:"label"`; Recipients []string `json:"recipients"`; ValidDays int `json:"validDays"`}
  if !readJSON(w,r,&data){return}
  details,raw,err:=s.store.NewAPIKey(owner,data.Label,data.Recipients,data.ValidDays)
  if err!=nil {
    if errors.Is(err,ErrInvalid){apiError(w,400,"请填写名称、有效期，并选择1-30位其他已注册用户");return}
    if errors.Is(err,ErrNotFound){apiError(w,404,"收件人不存在");return}
    if errors.Is(err,ErrExists){apiError(w,409,"每个账号最多创建5个 Key，请先撤销不用的 Key");return}
    apiError(w,500,"API Key 创建失败");return
  }
  writeJSON(w,201,map[string]any{"key":details,"secret":raw})
}
func (s *Server) revokeUserAPIKey(w http.ResponseWriter,r *http.Request,owner string){
  if err:=s.store.RevokeAPIKey(owner,r.PathValue("id"));err!=nil{
    if errors.Is(err,ErrNotFound){apiError(w,404,"Key 不存在或不属于你");return}
    apiError(w,500,"撤销 API Key 失败");return
  }
  writeJSON(w,200,map[string]bool{"ok":true})
}

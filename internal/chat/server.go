package chat

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Store          *Store
	InviteCode     string
	SessionSecret  string
	AllowedOrigins []string
	LANOriginCIDRs []string
}

type Server struct {
	store    *Store
	invite   string
	secret   []byte
	origins  map[string]bool
	lanOriginCIDRs []netip.Prefix
	hub      *Hub
	attempts sync.Mutex
	recent   map[string][]time.Time
	push *PushService
	native *NativePushService
	syncMu sync.Mutex
	historyRequests map[string]historyRequest
    sendMu sync.Mutex
    sendTimes map[string][]time.Time
}

func NewServer(c Config) (*Server, error) {
	if c.Store == nil || len(c.InviteCode) < 12 || len(c.SessionSecret) < 32 {
		return nil, errors.New("请设置至少12位邀请码和至少32位的 SESSION_SECRET")
	}
	origins := map[string]bool{}
	for _, v := range c.AllowedOrigins {
		v = strings.TrimRight(strings.TrimSpace(v), "/")
		if v == "" {
			continue
		}
		u, err := url.Parse(v)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, fmt.Errorf("非法允许来源: %s", v)
		}
		origins[v] = true
	}
    cidrs:=make([]netip.Prefix,0,len(c.LANOriginCIDRs))
    privateBases:=[]netip.Prefix{
        netip.MustParsePrefix("10.0.0.0/8"),
        netip.MustParsePrefix("172.16.0.0/12"),
        netip.MustParsePrefix("192.168.0.0/16"),
        netip.MustParsePrefix("fc00::/7"),
    }
    for _,entry:=range c.LANOriginCIDRs{
        entry=strings.TrimSpace(entry)
        if entry==""{continue}
        prefix,err:=netip.ParsePrefix(entry)
        if err!=nil{return nil,fmt.Errorf("invalid LAN CIDR: %q",entry)}
        prefix=prefix.Masked()
        safe:=false
        for _,base:=range privateBases {
            if prefix.Addr().Is4()==base.Addr().Is4()&&prefix.Bits()>=base.Bits()&&base.Contains(prefix.Addr()){
                safe=true
                break
            }
        }
        if !safe{return nil,fmt.Errorf("LAN CIDR outside private addresses: %q",entry)}
        cidrs=append(cidrs,prefix)
    }
	s := &Server{store: c.Store, invite: c.InviteCode, secret: []byte(c.SessionSecret), origins: origins, lanOriginCIDRs:cidrs, recent: map[string][]time.Time{},historyRequests:make(map[string]historyRequest),sendTimes:make(map[string][]time.Time)}
	s.hub = NewHub()
	return s, nil
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Server) originAllowed(r *http.Request) bool {
    return s.allowedOrigin(strings.TrimRight(r.Header.Get("Origin"),"/"))
}
func (s *Server) allowedOrigin(origin string) bool {
    if origin=="" || s.origins[origin] {return true}
    parsed,err:=url.Parse(origin)
    if err!=nil || (parsed.Scheme!="http" && parsed.Scheme!="https") ||
        parsed.User!=nil || parsed.RawQuery!="" || parsed.Fragment!="" ||
        parsed.Path!="" {return false}
    ip,err:=netip.ParseAddr(parsed.Hostname())
    if err!=nil || !ip.IsPrivate(){return false}
    for _,cidr:=range s.lanOriginCIDRs { if cidr.Contains(ip){return true} }
    return false
}

func (s *Server) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		o := strings.TrimRight(r.Header.Get("Origin"), "/")
		if o != "" {
			if !s.allowedOrigin(o) {
				// Do not weaken the allowlist: tell the client which configuration is missing.
                apiError(w, http.StatusForbidden, "访问来源未加入 ALLOWED_ORIGINS，检查域名、协议和端口；请在服务器 .env 添加当前网页的完整 Origin 后重启")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"app": "ZzzMessage", "version": "0.1.0", "messageStorage": "pending-until-ack", "maxPendingPerUser": maxPendingPerAccount, "pendingTTLDays": 14})
	})
	mux.HandleFunc("POST /api/auth/register", s.register)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("GET /api/me", s.auth(s.me))
	mux.HandleFunc("GET /api/users", s.auth(s.users))
	mux.HandleFunc("GET /api/rooms", s.auth(s.rooms))
	mux.HandleFunc("POST /api/rooms", s.auth(s.createGroup))
	mux.HandleFunc("POST /api/direct", s.auth(s.createDirect))
	mux.HandleFunc("GET /api/ws", s.websocket)
    mux.HandleFunc("GET /api/messages/pending",s.auth(s.httpPending))
    mux.HandleFunc("POST /api/messages/send",s.auth(s.httpSend))
    mux.HandleFunc("POST /api/messages/ack",s.auth(s.httpAck))
    mux.HandleFunc("GET /api/push/preferences",s.auth(s.getPushPreferences))
    mux.HandleFunc("PUT /api/push/preferences",s.auth(s.setPushPreferences))
	mux.HandleFunc("GET /api/money", s.auth(s.moneyBalance))
	mux.HandleFunc("POST /api/money/daily", s.auth(s.moneyDaily))
	mux.HandleFunc("GET /api/gifts", s.auth(s.giftCatalog))
	mux.HandleFunc("GET /api/gifts/history", s.auth(s.giftHistory))
	mux.HandleFunc("POST /api/gifts/send", s.auth(s.giftSend))
	mux.HandleFunc("GET /api/native-push/config", s.auth(s.nativePushConfig))
	mux.HandleFunc("PUT /api/native-push/device", s.auth(s.nativePushRegister))
	mux.HandleFunc("DELETE /api/native-push/device", s.auth(s.nativePushDelete))
	mux.HandleFunc("POST /api/native-push/test", s.auth(s.nativePushTest))
	mux.HandleFunc("GET /api/push/config", s.auth(s.pushConfig))
	mux.HandleFunc("PUT /api/push/subscription", s.auth(s.putPush))
	mux.HandleFunc("DELETE /api/push/subscription", s.auth(s.deletePush))
	mux.HandleFunc("POST /api/push/test", s.auth(s.testPush))
	mux.HandleFunc("GET /api/push/test-status", s.auth(s.pushTestStatus))
	return s.Middleware(mux)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
func apiError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func readJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		apiError(w, 415, "只接受 JSON")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		apiError(w, 400, "请求参数不合法")
		return false
	}
	if decoder.Decode(new(any)) != io.EOF {
		apiError(w, 400, "请求数据多余")
		return false
	}
	return true
}

func (s *Server) limited(r *http.Request) bool {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	now := time.Now()
	s.attempts.Lock()
	defer s.attempts.Unlock()
	// Small private server: 30 authentication requests per minute per transport IP.
	// Reverse proxies should enforce their own client-IP-aware rate limits.
	if len(s.recent) > 10000 {
		s.recent = map[string][]time.Time{}
	}
	times := s.recent[ip][:0]
	for _, t := range s.recent[ip] {
		if now.Sub(t) < time.Minute {
			times = append(times, t)
		}
	}
	if len(times) >= 30 {
		s.recent[ip] = times
		return true
	}
	s.recent[ip] = append(times, now)
	return false
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	if s.limited(r) {
		apiError(w, 429, "尝试次数过多")
		return
	}
	var p struct {
		Username    string `json:"username"`
		DisplayName string `json:"displayName"`
		Password    string `json:"password"`
		InviteCode  string `json:"inviteCode"`
	}
	if !readJSON(w, r, &p) {
		return
	}
	if subtle.ConstantTimeCompare([]byte(p.InviteCode), []byte(s.invite)) != 1 {
		apiError(w, 403, "邀请码错误")
		return
	}
	user, err := s.store.Register(p.Username, p.DisplayName, p.Password)
	if errors.Is(err, ErrExists) {
		apiError(w, 409, err.Error())
		return
	}
	if errors.Is(err, ErrInvalid) {
		apiError(w, 400, "用户名需为3-24位小写字母、数字或下划线，密码至少8位")
		return
	}
	if err != nil {
		log.Printf("register failed: %v", err)
		apiError(w, 500, "服务器错误")
		return
	}
	token, err := s.issue(user.Username)
	if err != nil {
		apiError(w, 500, "服务器错误")
		return
	}
    reward,err:=s.loginBonus(user.Username)
    if err!=nil{log.Printf("register bonus failed: %v",err);apiError(w,500,"每日奖励发放失败");return}
	writeJSON(w, 201, map[string]any{"user": user, "token": token,"dailyReward":reward})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.limited(r) {
		apiError(w, 429, "尝试次数过多")
		return
	}
	var p struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &p) {
		return
	}
	user, ok := s.store.Authenticate(p.Username, p.Password)
	if !ok {
		apiError(w, 401, "用户名或密码错误")
		return
	}
	token, err := s.issue(user.Username)
	if err != nil {
		apiError(w, 500, "服务器错误")
		return
	}
    reward,err:=s.loginBonus(user.Username)
    if err!=nil{log.Printf("login bonus failed: %v",err);apiError(w,500,"每日奖励发放失败");return}
	writeJSON(w, 200, map[string]any{"user": user, "token": token,"dailyReward":reward})
}

type session struct {
	Sub string `json:"sub"`
	Exp int64  `json:"exp"`
}

func (s *Server) sign(payload string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s *Server) issue(username string) (string, error) {
	body, err := json.Marshal(session{Sub: username, Exp: time.Now().Add(7 * 24 * time.Hour).Unix()})
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(body)
	return encoded + "." + s.sign(encoded), nil
}
func (s *Server) validate(token string) (string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || len(token) > 2048 {
		return "", false
	}
	actual, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	expected, err := base64.RawURLEncoding.DecodeString(s.sign(parts[0]))
	if err != nil || !hmac.Equal(actual, expected) {
		return "", false
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	var p session
	if json.Unmarshal(body, &p) != nil || p.Exp <= time.Now().Unix() {
		return "", false
	}
	if _, ok := s.store.GetUser(p.Sub); !ok {
		return "", false
	}
	return p.Sub, true
}

type securedHandler func(http.ResponseWriter, *http.Request, string)

func (s *Server) auth(next securedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value := r.Header.Get("Authorization")
		if !strings.HasPrefix(value, "Bearer ") {
			apiError(w, 401, "请先登录")
			return
		}
		username, ok := s.validate(strings.TrimPrefix(value, "Bearer "))
		if !ok {
			apiError(w, 401, "登录已过期")
			return
		}
		next(w, r, username)
	}
}
func (s *Server) me(w http.ResponseWriter, r *http.Request, u string) {
	user, _ := s.store.GetUser(u)
	writeJSON(w, 200, user)
}
func (s *Server) users(w http.ResponseWriter, r *http.Request, u string) {
	writeJSON(w, 200, s.store.Users(u))
}
func (s *Server) rooms(w http.ResponseWriter, r *http.Request, u string) {
	writeJSON(w, 200, s.store.RoomsFor(u))
}
func (s *Server) createDirect(w http.ResponseWriter, r *http.Request, u string) {
	var p struct {
		Username string `json:"username"`
	}
	if !readJSON(w, r, &p) {
		return
	}
	room, err := s.store.CreateDirect(u, strings.ToLower(strings.TrimSpace(p.Username)))
	if err != nil {
		apiError(w, 400, "无法创建私聊：用户不存在或参数错误")
		return
	}
	s.notifyRoomChanged(room)
	writeJSON(w, 201, room)
}
func (s *Server) createGroup(w http.ResponseWriter, r *http.Request, u string) {
	var p struct {
		Name    string   `json:"name"`
		Members []string `json:"members"`
	}
	if !readJSON(w, r, &p) {
		return
	}
	room, err := s.store.CreateGroup(u, p.Name, p.Members)
	if err != nil {
		apiError(w, 400, "群名称或成员无效（最多30人）")
		return
	}
	s.notifyRoomChanged(room)
	writeJSON(w, 201, room)
}

func (s *Server) notifyRoomChanged(room Room) {
	data, err := json.Marshal(map[string]string{"type": "rooms_changed"})
	if err == nil {
		s.hub.broadcast(room, data)
	}
}

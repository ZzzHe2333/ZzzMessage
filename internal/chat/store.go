package chat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
 webpush "github.com/SherClockHolmes/webpush-go"
)

type User struct {
	Username     string `json:"username"`
	DisplayName  string `json:"displayName"`
	PasswordHash string `json:"passwordHash"`
}

type PublicUser struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
}

func (u User) Public() PublicUser {
	return PublicUser{Username: u.Username, DisplayName: u.DisplayName}
}

type Room struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Members []string `json:"members"`
}

type diskState struct {
	Users map[string]User `json:"users"`
	Rooms map[string]Room `json:"rooms"`
 Push map[string][]webpush.Subscription `json:"pushSubscriptions,omitempty"`
}

type Store struct {
	mu    sync.RWMutex
	path  string
	state diskState
}

var ErrExists = errors.New("用户名已存在")
var ErrNotFound = errors.New("用户不存在")
var ErrInvalid = errors.New("参数不合法")

func NewStore(directory string) (*Store, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(directory, "state.json"), state: diskState{Users: make(map[string]User), Rooms: make(map[string]Room)}}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		s.state.Rooms["lobby"] = Room{ID: "lobby", Name: "好友大厅", Type: "lobby", Members: []string{}}
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(data, &s.state); err != nil {
		return nil, fmt.Errorf("读取账号数据失败: %w", err)
	}
	if s.state.Users == nil {
		s.state.Users = make(map[string]User)
	}
	if s.state.Rooms == nil {
		s.state.Rooms = make(map[string]Room)
	}
	if _, ok := s.state.Rooms["lobby"]; !ok {
		s.state.Rooms["lobby"] = Room{ID: "lobby", Name: "好友大厅", Type: "lobby", Members: []string{}}
	}
	return s, nil
}

func validUsername(u string) bool {
	if len(u) < 3 || len(u) > 24 {
		return false
	}
	for _, r := range u {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return true
}

func (s *Store) saveLocked() error {
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), s.path)
}

func (s *Store) Register(username, displayName, password string) (PublicUser, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	displayName = strings.TrimSpace(displayName)
	if !validUsername(username) || len(password) < 8 || len(password) > 72 || utf8.RuneCountInString(displayName) > 40 {
		return PublicUser{}, ErrInvalid
	}
	if displayName == "" {
		displayName = username
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return PublicUser{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.state.Users[username]; ok {
		return PublicUser{}, ErrExists
	}
	user := User{Username: username, DisplayName: displayName, PasswordHash: string(hash)}
	s.state.Users[username] = user
	if err = s.saveLocked(); err != nil {
		delete(s.state.Users, username)
		return PublicUser{}, err
	}
	return user.Public(), nil
}

func (s *Store) Authenticate(username, password string) (PublicUser, bool) {
	s.mu.RLock()
	u, ok := s.state.Users[strings.ToLower(strings.TrimSpace(username))]
	s.mu.RUnlock()
	if !ok || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return PublicUser{}, false
	}
	return u.Public(), true
}

func (s *Store) GetUser(username string) (PublicUser, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.state.Users[username]
	return u.Public(), ok
}

func (s *Store) Users(except string) []PublicUser {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]PublicUser, 0, len(s.state.Users))
	for _, u := range s.state.Users {
		if u.Username != except {
			result = append(result, u.Public())
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Username < result[j].Username })
	return result
}

func contains(a []string, x string) bool {
	for _, v := range a {
		if v == x {
			return true
		}
	}
	return false
}
func member(r Room, username string) bool { return r.Type == "lobby" || contains(r.Members, username) }

func (s *Store) RoomForUser(id, username string) (Room, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.state.Rooms[id]
	return r, ok && member(r, username)
}

func (s *Store) RoomsFor(username string) []Room {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Room, 0)
	for _, r := range s.state.Rooms {
		if member(r, username) {
			result = append(result, r)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID == "lobby" {
			return true
		}
		if result[j].ID == "lobby" {
			return false
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func (s *Store) CreateDirect(me, other string) (Room, error) {
	if me == other {
		return Room{}, ErrInvalid
	}
	members := []string{me, other}
	sort.Strings(members)
	key := sha256.Sum256([]byte(strings.Join(members, "\x00")))
	id := "dm_" + hex.EncodeToString(key[:12])
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.state.Users[other]; !ok {
		return Room{}, ErrNotFound
	}
	if r, ok := s.state.Rooms[id]; ok {
		return r, nil
	}
	r := Room{ID: id, Name: "", Type: "direct", Members: members}
	s.state.Rooms[id] = r
	if err := s.saveLocked(); err != nil {
		delete(s.state.Rooms, id)
		return Room{}, err
	}
	return r, nil
}

func (s *Store) CreateGroup(owner, name string, members []string) (Room, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 50 || len(members) > 30 {
		return Room{}, ErrInvalid
	}
	uniq := map[string]bool{owner: true}
	for _, m := range members {
		uniq[strings.ToLower(strings.TrimSpace(m))] = true
	}
	people := make([]string, 0, len(uniq))
	s.mu.Lock()
	defer s.mu.Unlock()
	for m := range uniq {
		if _, ok := s.state.Users[m]; !ok {
			return Room{}, ErrNotFound
		}
		people = append(people, m)
	}
	sort.Strings(people)
	id, err := randomID()
	if err != nil {
		return Room{}, err
	}
	r := Room{ID: "g_" + id, Name: name, Type: "group", Members: people}
	s.state.Rooms[r.ID] = r
	if err := s.saveLocked(); err != nil {
		delete(s.state.Rooms, r.ID)
		return Room{}, err
	}
	return r, nil
}

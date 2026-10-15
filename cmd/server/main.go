package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ZzzHe2333/ZzzMessage/internal/chat"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
func main() {
	store, err := chat.NewStore(env("DATA_DIR", "./data"))
	if err != nil {
		log.Fatal(err)
	}
	origins := strings.Split(env("ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:8080,http://127.0.0.1:5173,http://127.0.0.1:8080"), ",")
	app, err := chat.NewServer(chat.Config{Store: store, InviteCode: os.Getenv("INVITE_CODE"), SessionSecret: os.Getenv("SESSION_SECRET"), AllowedOrigins: origins})
	if err != nil {
		log.Fatal(err)
	}
	addr := env("LISTEN_ADDR", ":8080")
	server := &http.Server{Addr: addr, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 90 * time.Second}
	log.Printf("ZzzMessage API listening on %s; server stores accounts and rooms, not chat messages", addr)
	log.Fatal(server.ListenAndServe())
}

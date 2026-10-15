package main

import (
	"log"
	"bufio"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
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
	if err := loadEnvFile(".env"); err != nil { log.Fatal(err) }
	store, err := chat.NewStore(env("DATA_DIR", "./data"))
	if err != nil {
		log.Fatal(err)
	}
	origins := strings.Split(env("ALLOWED_ORIGINS", "http://localhost:58464,http://localhost:58463,http://127.0.0.1:58464,http://127.0.0.1:58463"), ",")
	app, err := chat.NewServer(chat.Config{Store: store, InviteCode: os.Getenv("INVITE_CODE"), SessionSecret: os.Getenv("SESSION_SECRET"), AllowedOrigins: origins, LANOriginCIDRs: strings.Split(env("LAN_ORIGIN_CIDRS","10.0.0.0/8,172.16.0.0/12,192.168.0.0/16"),",")})
	if err != nil {
		log.Fatal(err)
	}
	if err := app.ConfigureExternalSend(chat.ExternalSendConfig{
        Key: os.Getenv("EXTERNAL_SEND_KEY"),
        Sender: os.Getenv("EXTERNAL_SEND_SENDER"),
        AllowedRecipients: strings.Split(os.Getenv("EXTERNAL_SEND_RECIPIENTS"), ","),
    }); err != nil { log.Fatal(err) }
	if err := app.ConfigurePush(os.Getenv("VAPID_PUBLIC_KEY"), os.Getenv("VAPID_PRIVATE_KEY"), os.Getenv("VAPID_SUBJECT")); err != nil {
		log.Fatal(err)
	}
	if err := app.ConfigureNativePush(chat.NativePushConfig{
		XiaomiSecret: os.Getenv("XIAOMI_APP_SECRET"), XiaomiPackage: os.Getenv("XIAOMI_PACKAGE_NAME"),
		VivoAppID: os.Getenv("VIVO_APP_ID"), VivoAppKey: os.Getenv("VIVO_APP_KEY"), VivoAppSecret: os.Getenv("VIVO_APP_SECRET"),
	}); err != nil { log.Fatal(err) }
	addr := env("LISTEN_ADDR", ":58463")
	handler := app.Handler()
	if webDir := env("WEB_DIR", ""); webDir != "" {
		if info, err := os.Stat(webDir); err != nil || !info.IsDir() { log.Fatalf("WEB_DIR does not exist: %s", webDir) }
		handler = withStaticSite(handler, webDir)
		log.Printf("ZzzMessage static frontend: %s", webDir)
	}
	server := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 90 * time.Second}
	log.Printf("ZzzMessage API listening on %s; server persists pending messages until ACK (14 days / 500 per account)", addr)
	log.Fatal(server.ListenAndServe())
}


// loadEnvFile supports native Windows/Linux deployment without adding dependencies.
// Explicit environment settings override .env values.
var envName = regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_]*$")
func loadEnvFile(name string) error {
    file, err := os.Open(name)
    if errors.Is(err, os.ErrNotExist) { return nil }
    if err != nil { return err }
    defer file.Close()
    scanner := bufio.NewScanner(file)
    for line := 1; scanner.Scan(); line++ {
        raw := strings.TrimSpace(scanner.Text())
        if raw == "" || strings.HasPrefix(raw, "#") { continue }
        if strings.HasPrefix(raw, "export ") { raw = strings.TrimSpace(strings.TrimPrefix(raw, "export ")) }
        key, value, ok := strings.Cut(raw, "=")
        if !ok { return fmt.Errorf("invalid %s line %d", name, line) }
        key = strings.TrimSpace(key)
        if !envName.MatchString(key) { return fmt.Errorf("invalid environment key in %s line %d", name, line) }
        value = strings.TrimSpace(value)
        if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
            value = value[1:len(value)-1]
        }
        if _, existing := os.LookupEnv(key); !existing {
            if err := os.Setenv(key,value); err != nil { return err }
        }
    }
    return scanner.Err()
}

// Serve only precompiled Vue files. Keep the /api namespace on the API router.
func withStaticSite(api http.Handler, root string) http.Handler {
    static := http.FileServer(http.Dir(root))
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
            api.ServeHTTP(w,r)
            return
        }
        if r.Method != http.MethodGet && r.Method != http.MethodHead {
            http.Error(w,"Method not allowed",http.StatusMethodNotAllowed); return
        }
        cleaned := path.Clean("/" + r.URL.Path)
        if strings.HasPrefix(cleaned,"/.") || strings.Contains(cleaned,"/.") {
            http.NotFound(w,r); return
        }
        requested := filepath.Join(root,filepath.FromSlash(strings.TrimPrefix(cleaned,"/")))
        info,err:=os.Stat(requested)
        if err == nil && info.IsDir() {
            if _,e:=os.Stat(filepath.Join(requested,"index.html")); e!=nil {
                http.NotFound(w,r); return
            }
        }
        if errors.Is(err,os.ErrNotExist) {
            // SPA routes use index.html; missing .js/.css/.png assets return 404.
            if path.Ext(cleaned)!="" {http.NotFound(w,r); return}
            fallback:=r.Clone(r.Context())
            u:=*r.URL
            u.Path="/"
            fallback.URL=&u
            static.ServeHTTP(w,fallback)
            return
        }
        static.ServeHTTP(w,r)
    })
}
